// llmcaller.go – SeedClaw llm-caller core skill v2.1
//
// SINGLE SOURCE OF TRUTH: src/skills/core/llm-caller/SKILL.md, ARCHITECTURE.md v2.1, PRD.md v2.1
//
// Network policy (hard-coded, enforced at registration by seedclaw):
//   outbound: allow_list
//   domains:  api.openai.com, api.anthropic.com, grok.x.ai, ollama.ai, registry.ollama.ai
//   ports:    [443]
//   network_mode: seedclaw-net
//
// SECURITY INVARIANTS:
// - This skill NEVER listens on any port.
// - ALL communication routes exclusively via message-hub TCP connection.
// - Outbound HTTP calls limited to declared allow-list domains.
// - No filesystem mounts required; /tmp is a tmpfs.
//
// CONTROL_PLANE_PATTERN_v2.1.3: Client TCP retry – exponential backoff
// CONTROL_PLANE_PATTERN_v2.1.3: Active bufio.Scanner forever loop
// SECURITY_INVARIANT: PRD.md §3.2 – startup race mitigation

package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────
//
//	Approved outbound domains (enforced at runtime)
//
// SECURITY_INVARIANT: llm-caller/SKILL.md §Network Policy
// Any domain not on this list will cause the request to be rejected and an
// error message returned to the caller via message-hub.
// ─────────────────────────────────────────────
var approvedDomains = []string{
	"api.openai.com",
	"api.anthropic.com",
	"grok.x.ai",
	"ollama.ai",
	"registry.ollama.ai",
}

// Message is the canonical JSON envelope shared across the swarm.
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

// LLMRequest is the expected content.action=="call" payload from message-hub.
type LLMRequest struct {
	Action   string `json:"action"`
	Provider string `json:"provider"` // "openai" | "anthropic" | "ollama"
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	System   string `json:"system,omitempty"`
	// ReplyTo is the skill that should receive the response.
	ReplyTo       string `json:"reply_to,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// pendingOllama tracks in-flight sub-requests to the ollama skill.
// Key: sub-correlation ID used in the ollama generate message.
var (
	ollamaMu      sync.Mutex
	ollamaPending = map[string]chan string{}

	// writeMu serialises all writes to mainConn (TCP frames must not interleave).
	writeMu sync.Mutex
)

// mainConn is set once in main() and used by callOllamaSkill for sending.
var mainConn net.Conn

func main() {
	fmt.Fprintln(os.Stderr, "[llm-caller] starting …")

	conn := connectWithRetry()
	defer conn.Close()
	mainConn = conn

	// Register with message-hub so it knows we are up.
	sendRegistration(conn)

	fmt.Fprintln(os.Stderr, "[llm-caller] connected to message-hub, ready")

	// CONTROL_PLANE_PATTERN_v2.1.3: Active bufio.Scanner forever loop
	// SECURITY_INVARIANT: prevents all-goroutines-asleep deadlock
	// Each message is handled in its own goroutine so that blocking ollama relay
	// calls do not stall the scanner loop (which must stay unblocked to receive
	// the ollama response back through message-hub).
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Copy the slice – scanner reuses the buffer on the next Scan call.
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)
		go handleLine(conn, lineCopy)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[llm-caller] scanner error: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, "[llm-caller] connection closed, exiting")
}

// connectWithRetry dials message-hub with exponential back-off.
//
// CONTROL_PLANE_PATTERN_v2.1.3: Client TCP retry – exponential backoff
// SECURITY_INVARIANT: PRD.md §3.2 – startup race mitigation
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
		fmt.Fprintf(os.Stderr, "[llm-caller] hub not ready (attempt %d): %v – retrying in %s\n", attempt+1, err, delay)
		time.Sleep(delay)
	}
	fmt.Fprintf(os.Stderr, "[llm-caller] FATAL: could not connect to message-hub: %v\n", err)
	os.Exit(1)
	return nil
}

func sendRegistration(conn net.Conn) {
	payload, _ := json.Marshal(map[string]string{
		"action": "register",
		"skill":  "llm-caller",
	})
	msg := Message{From: "llm-caller", To: "message-hub", Content: payload}
	sendMsg(conn, msg)
}

func handleLine(conn net.Conn, raw []byte) {
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		fmt.Fprintf(os.Stderr, "[llm-caller] bad JSON: %v\n", err)
		return
	}

	// Route ollama generate/chat responses back to the waiting callOllamaSkill goroutine.
	if msg.From == "ollama" {
		var content map[string]string
		if err := json.Unmarshal(msg.Content, &content); err == nil {
			action := content["action"]
			if action == "generate_response" || action == "chat_response" || action == "ollama_error" {
				subID := content["correlation_id"]
				ollamaMu.Lock()
				ch, ok := ollamaPending[subID]
				if ok {
					delete(ollamaPending, subID)
				}
				ollamaMu.Unlock()
				if ok {
					if action == "ollama_error" {
						ch <- "ERROR:" + content["error"]
					} else {
						ch <- content["response"]
					}
				}
				return
			}
		}
	}

	// Handle route_error from message-hub (e.g. target skill not yet connected).
	// If this is a route_error for a pending ollama request, signal the channel so
	// callOllamaSkill can retry.
	if msg.From == "message-hub" {
		var content map[string]string
		if err := json.Unmarshal(msg.Content, &content); err == nil && content["action"] == "route_error" {
			// Route errors for pending ollama requests use the correlation_id.
			// Since route_errors don't carry correlation_id, we signal ALL pending channels.
			// callOllamaSkill treats "RETRY:" as a cue to re-send the message.
			ollamaMu.Lock()
			for subID, ch := range ollamaPending {
				delete(ollamaPending, subID)
				ch <- "RETRY:" + content["error"]
			}
			ollamaMu.Unlock()
			fmt.Fprintf(os.Stderr, "[llm-caller] route_error from message-hub: %s\n", content["error"])
			return
		}
	}

	if msg.To != "llm-caller" {
		return
	}

	var req LLMRequest
	if err := json.Unmarshal(msg.Content, &req); err != nil {
		sendError(conn, msg.From, req.CorrelationID, "invalid llm request: "+err.Error())
		return
	}
	if req.Action != "call" {
		sendError(conn, msg.From, req.CorrelationID, "unknown action: "+req.Action)
		return
	}

	result, err := dispatchLLMCall(req)
	if err != nil {
		sendError(conn, msg.From, req.CorrelationID, err.Error())
		return
	}

	replyTo := msg.From
	if req.ReplyTo != "" {
		replyTo = req.ReplyTo
	}
	payload, _ := json.Marshal(map[string]string{
		"action":         "llm_response",
		"response":       result,
		"correlation_id": req.CorrelationID,
	})
	sendMsg(conn, Message{From: "llm-caller", To: replyTo, Content: payload})
}

// dispatchLLMCall routes the request to the appropriate provider.
// SECURITY_INVARIANT: outbound only to approved domains.
func dispatchLLMCall(req LLMRequest) (string, error) {
	switch strings.ToLower(req.Provider) {
	case "openai":
		return callOpenAI(req)
	case "anthropic":
		return callAnthropic(req)
	case "ollama":
		return callOllamaSkill(req)
	default:
		return "", fmt.Errorf("unsupported provider %q", req.Provider)
	}
}

// domainAllowed returns true if the given URL host is in the approved list.
// SECURITY_INVARIANT: llm-caller/SKILL.md §Network Policy – outbound allow_list.
func domainAllowed(host string) bool {
	for _, d := range approvedDomains {
		if strings.EqualFold(host, d) {
			return true
		}
	}
	return false
}

func callOpenAI(req LLMRequest) (string, error) {
	host := "api.openai.com"
	if !domainAllowed(host) {
		return "", fmt.Errorf("SECURITY: domain %q is not on allow-list", host)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY not set")
	}

	model := req.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	messages := buildMessages(req)
	body, _ := json.Marshal(map[string]interface{}{
		"model":    model,
		"messages": messages,
	})

	httpReq, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("OpenAI request failed: %w", err)
	}
	defer resp.Body.Close()
	return parseOpenAIResponse(resp.Body)
}

func callAnthropic(req LLMRequest) (string, error) {
	host := "api.anthropic.com"
	if !domainAllowed(host) {
		return "", fmt.Errorf("SECURITY: domain %q is not on allow-list", host)
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	model := req.Model
	if model == "" {
		model = "claude-3-haiku-20240307"
	}

	messages := []map[string]string{{"role": "user", "content": req.Prompt}}
	payload := map[string]interface{}{
		"model":      model,
		"max_tokens": 4096,
		"messages":   messages,
	}
	if req.System != "" {
		payload["system"] = req.System
	}

	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("Anthropic request failed: %w", err)
	}
	defer resp.Body.Close()
	return parseAnthropicResponse(resp.Body)
}

// callOllamaSkill relays generation requests to the ollama skill via message-hub.
// It uses a per-request channel to receive the async response from the scanner loop.
func callOllamaSkill(req LLMRequest) (string, error) {
	model := req.Model
	if model == "" {
		model = "nemotron-3-nano:latest"
		if m := os.Getenv("OLLAMA_DEFAULT_MODEL"); m != "" {
			model = m
		}
	}

	const maxRetries = 20
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(500*(1<<uint(attempt-1))) * time.Millisecond
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			fmt.Fprintf(os.Stderr, "[llm-caller] ollama not ready, retry %d in %s\n", attempt, delay)
			time.Sleep(delay)
		}

		// Generate a sub-correlation ID for the ollama generate message.
		b := make([]byte, 8)
		rand.Read(b) //nolint:errcheck
		subID := hex.EncodeToString(b)

		ch := make(chan string, 1)
		ollamaMu.Lock()
		ollamaPending[subID] = ch
		ollamaMu.Unlock()

		payload, _ := json.Marshal(map[string]string{
			"action":         "generate",
			"model":          model,
			"prompt":         req.Prompt,
			"correlation_id": subID,
		})
		sendMsg(mainConn, Message{From: "llm-caller", To: "ollama", Content: payload})

		// Wait for the response (up to 5 minutes for large models).
		select {
		case result := <-ch:
			if strings.HasPrefix(result, "RETRY:") {
				// message-hub told us the skill wasn't connected – retry.
				continue
			}
			if strings.HasPrefix(result, "ERROR:") {
				return "", fmt.Errorf("ollama: %s", strings.TrimPrefix(result, "ERROR:"))
			}
			return result, nil
		case <-time.After(300 * time.Second):
			ollamaMu.Lock()
			delete(ollamaPending, subID)
			ollamaMu.Unlock()
			return "", fmt.Errorf("ollama request timed out after 300s")
		}
	}
	return "", fmt.Errorf("ollama not available after %d retries", maxRetries)
}

// ─────────────────────────────────────────────
//  Response parsers
// ─────────────────────────────────────────────

func parseOpenAIResponse(body io.Reader) (string, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	data, _ := io.ReadAll(body)
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse error: %w – body: %s", err, data)
	}
	if resp.Error.Message != "" {
		return "", fmt.Errorf("OpenAI error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from OpenAI")
	}
	return resp.Choices[0].Message.Content, nil
}

func parseAnthropicResponse(body io.Reader) (string, error) {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	data, _ := io.ReadAll(body)
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse error: %w – body: %s", err, data)
	}
	if resp.Error.Message != "" {
		return "", fmt.Errorf("Anthropic error: %s", resp.Error.Message)
	}
	for _, c := range resp.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text content returned from Anthropic")
}

// ─────────────────────────────────────────────
//  Helpers
// ─────────────────────────────────────────────

func buildMessages(req LLMRequest) []map[string]string {
	var messages []map[string]string
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.Prompt})
	return messages
}

func sendError(conn net.Conn, to, correlID, detail string) {
	payload, _ := json.Marshal(map[string]string{
		"action":         "llm_error",
		"error":          detail,
		"correlation_id": correlID,
	})
	sendMsg(conn, Message{From: "llm-caller", To: to, Content: payload})
}

func sendMsg(conn net.Conn, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[llm-caller] marshal error: %v\n", err)
		return
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	if _, err := fmt.Fprintf(conn, "%s\n", data); err != nil {
		fmt.Fprintf(os.Stderr, "[llm-caller] send error: %v\n", err)
	}
}
