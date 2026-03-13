// ollama.go – SeedClaw ollama core skill v2.1.4
//
// SINGLE SOURCE OF TRUTH: src/skills/core/ollama/SKILL.md v2.1.4, ARCHITECTURE.md v2.1, PRD.md v2.1
//
// CRITICAL DUAL-PROCESS MODEL (NON-NEGOTIABLE):
// This Go binary MUST be PID 1 (ENTRYPOINT). It:
//  1. Starts `ollama serve` as a child process bound to 127.0.0.1 only.
//  2. Connects to message-hub via TCP (retry + bufio.Scanner pattern).
//  3. Bridges ALL message-hub requests → http://127.0.0.1:11434 → replies.
//  4. Propagates SIGTERM to the child on shutdown.
//  5. Monitors child health periodically.
//
// SECURITY INVARIANTS:
// - Ollama HTTP API MUST bind to 127.0.0.1:11434 ONLY (never 0.0.0.0).
// - No EXPOSE 11434 in Dockerfile; no ports: in compose.yaml.
// - No other skill may reach port 11434 directly.
// - Only outbound: registry.ollama.ai for model pulls (SKILL.md network_policy).
//
// CONTROL_PLANE_PATTERN_v2.1.3: Client TCP retry – exponential backoff
// CONTROL_PLANE_PATTERN_v2.1.3: Active bufio.Scanner forever loop
// SECURITY_INVARIANT: PRD.md §3.2 – startup race mitigation

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// Message is the canonical JSON envelope.
type Message struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	Content  json.RawMessage `json:"content"`
	Metadata MsgMeta         `json:"metadata,omitempty"`
}

type MsgMeta struct {
	SenderValidated bool   `json:"sender_validated,omitempty"`
	PolicyHash      string `json:"policy_hash,omitempty"`
}

// OllamaRequest describes the expected content for Ollama-directed messages.
type OllamaRequest struct {
	Action        string `json:"action"` // pull | generate | chat | list
	Model         string `json:"model,omitempty"`
	Prompt        string `json:"prompt,omitempty"`
	Stream        bool   `json:"stream,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// ─────────────────────────────────────────────
//
//	Internal Ollama client
//	SECURITY_INVARIANT: only talks to 127.0.0.1:11434 – never leaves container.
//
// ─────────────────────────────────────────────
const ollamaBaseURL = "http://127.0.0.1:11434"

var httpClient = &http.Client{Timeout: 300 * time.Second}

// ─────────────────────────────────────────────
//  main
// ─────────────────────────────────────────────

func main() {
	fmt.Fprintln(os.Stderr, "[ollama-skill] starting (Go supervisor / PID 1) …")

	// Step 1: start the real Ollama server as a child process.
	// SECURITY_INVARIANT: ollama/SKILL.md v2.1.4 – bind to 127.0.0.1 only.
	ollamaCmd := startOllamaChild()

	// Handle SIGTERM/SIGINT – gracefully stop child before exiting.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigs
		fmt.Fprintf(os.Stderr, "[ollama-skill] received signal %v – stopping child\n", sig)
		if ollamaCmd.Process != nil {
			ollamaCmd.Process.Signal(syscall.SIGTERM) //nolint:errcheck
		}
		os.Exit(0)
	}()

	// Step 2: wait for child Ollama to be responsive.
	waitForOllama()

	// Step 3: Connect to message-hub.
	//
	// CONTROL_PLANE_PATTERN_v2.1.3: Client TCP retry – exponential backoff
	// SECURITY_INVARIANT: PRD.md §3.2 – startup race mitigation
	conn := connectWithRetry()
	defer conn.Close()

	// Step 4: Register with message-hub.
	sendRegistration(conn)
	fmt.Fprintln(os.Stderr, "[ollama-skill] connected to message-hub, ready")

	// Step 5: Health monitor in background.
	go healthMonitor(conn, ollamaCmd)

	// Step 6: Message loop.
	//
	// CONTROL_PLANE_PATTERN_v2.1.3: Active bufio.Scanner forever loop
	// SECURITY_INVARIANT: prevents all-goroutines-asleep deadlock
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		handleLine(conn, line)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[ollama-skill] scanner error: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, "[ollama-skill] connection closed – stopping child")
	if ollamaCmd.Process != nil {
		ollamaCmd.Process.Signal(syscall.SIGTERM) //nolint:errcheck
	}
}

// ─────────────────────────────────────────────
//  Child process management
// ─────────────────────────────────────────────

// startOllamaChild launches `ollama serve` bound to loopback only.
// SECURITY_INVARIANT: ollama/SKILL.md v2.1.4 – Ollama MUST bind to 127.0.0.1 only.
func startOllamaChild() *exec.Cmd {
	cmd := exec.Command("ollama", "serve")
	cmd.Env = append(os.Environ(), "OLLAMA_HOST=127.0.0.1")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[ollama-skill] FATAL: cannot start ollama child: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[ollama-skill] ollama child started (PID %d)\n", cmd.Process.Pid)
	return cmd
}

// waitForOllama polls /api/tags until Ollama is responsive (up to 60 s).
func waitForOllama() {
	for i := 0; i < 60; i++ {
		resp, err := httpClient.Get(ollamaBaseURL + "/api/tags")
		if err == nil {
			resp.Body.Close()
			fmt.Fprintln(os.Stderr, "[ollama-skill] ollama child is responsive")
			return
		}
		time.Sleep(1 * time.Second)
	}
	fmt.Fprintln(os.Stderr, "[ollama-skill] WARNING: ollama child did not respond within 60 s – continuing")
}

// healthMonitor periodically checks that the child is still alive.
func healthMonitor(conn net.Conn, cmd *exec.Cmd) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		resp, err := httpClient.Get(ollamaBaseURL + "/api/tags")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ollama-skill] child health check failed: %v\n", err)
			sendAuditEvent(conn, "ollama_health_fail", "warn", err.Error())
		} else {
			resp.Body.Close()
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			sendAuditEvent(conn, "ollama_child_exited", "fatal", "child process exited unexpectedly")
			fmt.Fprintln(os.Stderr, "[ollama-skill] FATAL: ollama child exited")
			os.Exit(1)
		}
	}
}

// ─────────────────────────────────────────────
//  message-hub client
// ─────────────────────────────────────────────

// connectWithRetry dials message-hub with exponential back-off.
//
// CONTROL_PLANE_PATTERN_v2.1.3: Client TCP retry – exponential backoff
func connectWithRetry() net.Conn {
	hubAddr := os.Getenv("HUB_ADDR")
	if hubAddr == "" {
		hubAddr = "message-hub:8765"
	}
	var (
		conn net.Conn
		err  error
	)
	for attempt := 0; attempt < 30; attempt++ {
		conn, err = net.Dial("tcp", hubAddr)
		if err == nil {
			return conn
		}
		delay := time.Duration(500*(1<<uint(attempt))) * time.Millisecond
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		fmt.Fprintf(os.Stderr, "[ollama-skill] hub not ready (attempt %d): %v – retry in %s\n", attempt+1, err, delay)
		time.Sleep(delay)
	}
	fmt.Fprintf(os.Stderr, "[ollama-skill] FATAL: could not connect to message-hub: %v\n", err)
	os.Exit(1)
	return nil
}

func sendRegistration(conn net.Conn) {
	payload, _ := json.Marshal(map[string]string{"action": "register", "skill": "ollama"})
	sendMsg(conn, Message{From: "ollama", To: "message-hub", Content: payload})
}

// ─────────────────────────────────────────────
//  Message handling
// ─────────────────────────────────────────────

func handleLine(conn net.Conn, raw []byte) {
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		fmt.Fprintf(os.Stderr, "[ollama-skill] bad JSON: %v\n", err)
		return
	}
	if msg.To != "ollama" {
		return
	}

	var req OllamaRequest
	if err := json.Unmarshal(msg.Content, &req); err != nil {
		sendError(conn, msg.From, "", "invalid ollama request: "+err.Error())
		return
	}

	switch req.Action {
	case "generate":
		handleGenerate(conn, msg.From, req)
	case "chat":
		handleChat(conn, msg.From, req)
	case "pull":
		handlePull(conn, msg.From, req)
	case "list":
		handleList(conn, msg.From)
	default:
		sendError(conn, msg.From, req.CorrelationID, "unknown action: "+req.Action)
	}

	sendAuditEvent(conn, "ollama_bridge_call", "ok",
		fmt.Sprintf("action=%q model=%q from=%q", req.Action, req.Model, msg.From))
}

func handleGenerate(conn net.Conn, replyTo string, req OllamaRequest) {
	body, _ := json.Marshal(map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"stream": false,
	})
	resp, err := httpClient.Post(ollamaBaseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		sendError(conn, replyTo, req.CorrelationID, "ollama generate error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	data, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(data, &result); err != nil {
		sendError(conn, replyTo, req.CorrelationID, "parse error: "+err.Error())
		return
	}
	if resp.StatusCode != http.StatusOK {
		errMsg, _ := result["error"].(string)
		if errMsg == "" {
			errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		sendError(conn, replyTo, req.CorrelationID, "ollama error: "+errMsg)
		return
	}
	response, _ := result["response"].(string)
	payload, _ := json.Marshal(map[string]string{"action": "generate_response", "response": response, "correlation_id": req.CorrelationID})
	sendMsg(conn, Message{From: "ollama", To: replyTo, Content: payload})
}

func handleChat(conn net.Conn, replyTo string, req OllamaRequest) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "user", "content": req.Prompt},
		},
		"stream": false,
	})
	resp, err := httpClient.Post(ollamaBaseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		sendError(conn, replyTo, req.CorrelationID, "ollama chat error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	data, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(data, &result); err != nil {
		sendError(conn, replyTo, req.CorrelationID, "parse error: "+err.Error())
		return
	}
	if resp.StatusCode != http.StatusOK {
		errMsg, _ := result["error"].(string)
		if errMsg == "" {
			errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		sendError(conn, replyTo, req.CorrelationID, "ollama error: "+errMsg)
		return
	}

	var content string
	if m, ok := result["message"].(map[string]interface{}); ok {
		content, _ = m["content"].(string)
	}
	payload, _ := json.Marshal(map[string]string{"action": "chat_response", "response": content, "correlation_id": req.CorrelationID})
	sendMsg(conn, Message{From: "ollama", To: replyTo, Content: payload})
}

// handlePull requests a model pull from registry.ollama.ai.
// SECURITY_INVARIANT: only outbound domain allowed is registry.ollama.ai (SKILL.md network_policy).
func handlePull(conn net.Conn, replyTo string, req OllamaRequest) {
	body, _ := json.Marshal(map[string]interface{}{"name": req.Model, "stream": false})
	resp, err := httpClient.Post(ollamaBaseURL+"/api/pull", "application/json", bytes.NewReader(body))
	if err != nil {
		sendError(conn, replyTo, req.CorrelationID, "ollama pull error: "+err.Error())
		return
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &result) //nolint:errcheck
	status, _ := result["status"].(string)
	payload, _ := json.Marshal(map[string]string{"action": "pull_response", "status": status})
	sendMsg(conn, Message{From: "ollama", To: replyTo, Content: payload})
}

func handleList(conn net.Conn, replyTo string) {
	resp, err := httpClient.Get(ollamaBaseURL + "/api/tags")
	if err != nil {
		sendError(conn, replyTo, "", "ollama list error: "+err.Error())
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	sendMsg(conn, Message{From: "ollama", To: replyTo, Content: data})
}

// ─────────────────────────────────────────────
//  Helpers
// ─────────────────────────────────────────────

func sendAuditEvent(conn net.Conn, action, status, detail string) {
	payload, _ := json.Marshal(map[string]string{
		"action": "audit_event",
		"event":  action,
		"status": status,
		"detail": detail,
	})
	sendMsg(conn, Message{From: "ollama", To: "message-hub", Content: payload})
}

func sendError(conn net.Conn, to, correlID, detail string) {
	payload, _ := json.Marshal(map[string]string{"action": "ollama_error", "error": detail, "correlation_id": correlID})
	sendMsg(conn, Message{From: "ollama", To: to, Content: payload})
}

func sendMsg(conn net.Conn, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ollama-skill] marshal error: %v\n", err)
		return
	}
	if _, err := fmt.Fprintf(conn, "%s\n", data); err != nil {
		fmt.Fprintf(os.Stderr, "[ollama-skill] send error: %v\n", err)
	}
}
