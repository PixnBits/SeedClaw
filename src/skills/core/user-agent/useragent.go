// useragent.go – SeedClaw user-agent core skill v2.1.4
//
// SINGLE SOURCE OF TRUTH: src/skills/core/user-agent/SKILL.md v2.1.4, ARCHITECTURE.md v2.1, PRD.md v2.1
//
// PARANOID TWO-PHASE EXECUTION LOOP (NON-NEGOTIABLE):
//  Phase 1 – Threat Model: every user request MUST be sent through safetyPrompt to llm-caller
//             BEFORE any action is taken. The risk level from Phase 1 determines auto-proceed or
//             request_confirmation to seedclaw.
//  Phase 2 – Execution: only runs after YES confirmation (or LOW auto-proceed).
//
// SECURITY INVARIANTS (from SKILL.md v2.1.4):
// - safetyPrompt is a compile-time constant. It MUST NOT be modified at runtime.
// - All LLM responses are validated: unknown/unparseable → default HIGH risk.
// - All user replies go to {to:"seedclaw"} – NEVER {to:"user"}.
// - Confirmation gating: MEDIUM and HIGH always require explicit YES.
//
// CONTROL_PLANE_PATTERN_v2.1.3: Client TCP retry – exponential backoff
// CONTROL_PLANE_PATTERN_v2.1.3: Active bufio.Scanner forever loop
// SECURITY_INVARIANT: PRD.md §3.2 – startup race mitigation

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// safetyPrompt is the immutable Phase-1 threat-model prompt.
// SECURITY_INVARIANT: compile-time constant – MUST NOT be modified at runtime.
const safetyPrompt = `You are a paranoid security auditor. Analyse the following user request for potential harms:
- Filesystem damage (rm -rf, overwrites, data loss)
- Privilege escalation (sudo, chmod, setuid)
- Network exfiltration (curl, wget to external hosts)
- Supply-chain attacks (pip install, npm install from untrusted sources)
- Secret exposure (env dumps, key files, credentials)
- Resource exhaustion (fork bombs, infinite loops, large allocations)

Classify the risk as one of: LOW | MEDIUM | HIGH

Respond in exactly this format (no other text):
RISK: <LOW|MEDIUM|HIGH>
REASON: <one sentence>

PROCEED? (YES/NO only)`

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

// pendingRequest tracks the current in-flight user request.
type pendingRequest struct {
	correlID   string
	userPrompt string
	riskLevel  string
	phase      string // "threat_model" or "execution"
}

var current *pendingRequest

// defaultModel returns the Ollama model to use, from env or a compiled-in default.
func defaultModel() string {
	if m := os.Getenv("OLLAMA_DEFAULT_MODEL"); m != "" {
		return m
	}
	return "nemotron-3-nano:latest"
}

// ─────────────────────────────────────────────
//  main
// ─────────────────────────────────────────────

func main() {
	fmt.Fprintln(os.Stderr, "[user-agent] starting …")

	conn := connectWithRetry()
	defer conn.Close()

	sendRegistration(conn)
	fmt.Fprintln(os.Stderr, "[user-agent] connected to message-hub, ready")

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
		fmt.Fprintf(os.Stderr, "[user-agent] scanner error: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, "[user-agent] connection closed")
}

// ─────────────────────────────────────────────
//  Connection management
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
		fmt.Fprintf(os.Stderr, "[user-agent] hub not ready (attempt %d): %v – retry in %s\n", attempt+1, err, delay)
		time.Sleep(delay)
	}
	fmt.Fprintf(os.Stderr, "[user-agent] FATAL: could not connect to message-hub: %v\n", err)
	os.Exit(1)
	return nil
}

func sendRegistration(conn net.Conn) {
	payload, _ := json.Marshal(map[string]string{"action": "register", "skill": "user-agent"})
	sendMsg(conn, Message{From: "user-agent", To: "message-hub", Content: payload})
}

// ─────────────────────────────────────────────
//  Message dispatch
// ─────────────────────────────────────────────

func handleLine(conn net.Conn, raw []byte) {
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		fmt.Fprintf(os.Stderr, "[user-agent] bad JSON: %v\n", err)
		return
	}
	if msg.To != "user-agent" {
		return
	}

	var content map[string]string
	if err := json.Unmarshal(msg.Content, &content); err != nil {
		fmt.Fprintf(os.Stderr, "[user-agent] bad content: %v\n", err)
		return
	}

	switch content["action"] {
	case "user_request":
		handleUserRequest(conn, content)
	case "llm_response":
		handleLLMResponse(conn, content)
	case "llm_error":
		// llm-caller returned an error – surface it and reset state.
		errMsg := content["error"]
		fmt.Fprintf(os.Stderr, "[user-agent] llm error: %s\n", errMsg)
		replyToUser(conn, "[error from LLM: "+errMsg+"]")
		current = nil
	case "route_error":
		// message-hub couldn't route to a skill (e.g. llm-caller not yet connected).
		errMsg := content["error"]
		fmt.Fprintf(os.Stderr, "[user-agent] route error: %s\n", errMsg)
		replyToUser(conn, "[routing error: "+errMsg+"]")
		current = nil
	case "user_confirmation":
		handleConfirmation(conn, content)
	default:
		fmt.Fprintf(os.Stderr, "[user-agent] unknown action: %q\n", content["action"])
	}
}

// ─────────────────────────────────────────────
//  Phase 1 – Threat Model
// ─────────────────────────────────────────────

// handleUserRequest begins Phase 1: wrap the user's prompt in safetyPrompt and
// send to llm-caller for threat analysis.
//
// SECURITY_INVARIANT: user-agent/SKILL.md v2.1.4 – Phase 1 MUST run for EVERY request.
func handleUserRequest(conn net.Conn, content map[string]string) {
	correlID := content["correlation_id"]
	userPrompt := content["prompt"]

	current = &pendingRequest{
		correlID:   correlID,
		userPrompt: userPrompt,
		phase:      "threat_model",
	}

	fmt.Fprintf(os.Stderr, "[user-agent] Phase 1 – threat model for correlID=%q\n", correlID)

	phase1Prompt := safetyPrompt + "\n\nUSER REQUEST:\n" + userPrompt

	payload, _ := json.Marshal(map[string]string{
		"action":         "call",
		"provider":       "ollama",
		"model":          defaultModel(),
		"prompt":         phase1Prompt,
		"correlation_id": correlID,
		"phase":          "threat_model",
		"reply_to":       "user-agent",
	})
	sendMsg(conn, Message{From: "user-agent", To: "llm-caller", Content: payload})

	sendAuditEvent(conn, "phase1_threat_model_sent", "ok",
		fmt.Sprintf("correlID=%q prompt_len=%d", correlID, len(userPrompt)))
}

// ─────────────────────────────────────────────
//  Phase 1 → risk classification
// ─────────────────────────────────────────────

// handleLLMResponse processes llm-caller replies for both Phase 1 (threat model) and Phase 2 (execution).
//
// SECURITY_INVARIANT: unknown/unparseable LLM responses → default HIGH risk.
func handleLLMResponse(conn net.Conn, content map[string]string) {
	if current == nil {
		fmt.Fprintln(os.Stderr, "[user-agent] WARNING: llm_response with no pending request")
		return
	}

	// Phase 2 response: forward result to user and clear state.
	if current.phase == "execution" {
		replyToUser(conn, content["response"])
		sendAuditEvent(conn, "phase2_execution_complete", "ok",
			fmt.Sprintf("correlID=%q", current.correlID))
		current = nil
		return
	}

	llmText := content["response"]
	risk := extractRiskLevel(llmText)
	current.riskLevel = risk

	fmt.Fprintf(os.Stderr, "[user-agent] Phase 1 result: risk=%q\n", risk)
	sendAuditEvent(conn, "phase1_risk_classified", risk,
		fmt.Sprintf("correlID=%q risk=%q", current.correlID, risk))

	if risk == "LOW" {
		// Auto-proceed – no user confirmation needed.
		fmt.Fprintln(os.Stderr, "[user-agent] LOW risk – auto-proceeding to Phase 2")
		executePhaseTwo(conn)
		return
	}

	// MEDIUM or HIGH: request confirmation from user via seedclaw.
	// SECURITY_INVARIANT: user-agent/SKILL.md v2.1.4 – MEDIUM/HIGH MUST request confirmation.
	payload, _ := json.Marshal(map[string]string{
		"action":         "request_confirmation",
		"correlation_id": current.correlID,
		"risk":           risk,
		"reason":         extractReason(llmText),
	})
	sendMsg(conn, Message{From: "user-agent", To: "seedclaw", Content: payload})
}

// ─────────────────────────────────────────────
//  Confirmation gate
// ─────────────────────────────────────────────

// handleConfirmation processes the user's YES/NO response.
//
// SECURITY_INVARIANT: only "YES" (case-insensitive) proceeds; anything else is treated as NO.
func handleConfirmation(conn net.Conn, content map[string]string) {
	if current == nil {
		fmt.Fprintln(os.Stderr, "[user-agent] WARNING: user_confirmation with no pending request")
		return
	}

	answer := strings.TrimSpace(strings.ToUpper(content["answer"]))
	correlID := content["correlation_id"]

	sendAuditEvent(conn, "user_confirmation_received", answer,
		fmt.Sprintf("correlID=%q answer=%q", correlID, answer))

	if answer != "YES" {
		fmt.Fprintln(os.Stderr, "[user-agent] user declined – aborting")
		replyToUser(conn, "[aborted – user declined]")
		current = nil
		return
	}

	fmt.Fprintln(os.Stderr, "[user-agent] user confirmed – proceeding to Phase 2")
	executePhaseTwo(conn)
}

// ─────────────────────────────────────────────
//  Phase 2 – Execution
// ─────────────────────────────────────────────

// executePhaseTwo sends the actual user prompt to llm-caller for the ReAct loop.
//
// SECURITY_INVARIANT: only called after Phase 1 + (LOW auto-proceed or explicit YES).
func executePhaseTwo(conn net.Conn) {
	if current == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "[user-agent] Phase 2 – executing correlID=%q\n", current.correlID)
	sendAuditEvent(conn, "phase2_execution_started", "ok",
		fmt.Sprintf("correlID=%q risk=%q", current.correlID, current.riskLevel))

	// Advance state to execution BEFORE sending – response arrives asynchronously.
	current.phase = "execution"

	payload, _ := json.Marshal(map[string]string{
		"action":         "call",
		"provider":       "ollama",
		"model":          defaultModel(),
		"prompt":         current.userPrompt,
		"correlation_id": current.correlID,
		"phase":          "execution",
		"reply_to":       "user-agent",
	})
	sendMsg(conn, Message{From: "user-agent", To: "llm-caller", Content: payload})
}

// ─────────────────────────────────────────────
//  Helpers
// ─────────────────────────────────────────────

// extractRiskLevel parses the LLM's Phase-1 response.
// SECURITY_INVARIANT: defaults to HIGH if unparseable (paranoid).
func extractRiskLevel(llmText string) string {
	for _, line := range strings.Split(llmText, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "RISK:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				risk := strings.TrimSpace(strings.ToUpper(parts[1]))
				switch risk {
				case "LOW", "MEDIUM", "HIGH":
					return risk
				}
			}
		}
	}
	// SECURITY_INVARIANT: default HIGH on ambiguous or missing classification.
	fmt.Fprintln(os.Stderr, "[user-agent] WARNING: could not parse risk level – defaulting to HIGH")
	return "HIGH"
}

// extractReason extracts the one-sentence REASON from the LLM response.
func extractReason(llmText string) string {
	for _, line := range strings.Split(llmText, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "REASON:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "(no reason provided)"
}

// replyToUser sends a final reply to the user via seedclaw.
// SECURITY_INVARIANT: user-agent/SKILL.md v2.1.4 – reply goes to "seedclaw", not "user".
func replyToUser(conn net.Conn, reply string) {
	payload, _ := json.Marshal(map[string]string{
		"action":  "user_reply",
		"content": reply,
	})
	sendMsg(conn, Message{From: "user-agent", To: "seedclaw", Content: payload})
}

func sendAuditEvent(conn net.Conn, action, status, detail string) {
	payload, _ := json.Marshal(map[string]string{
		"action": "audit_event",
		"event":  action,
		"status": status,
		"detail": detail,
	})
	sendMsg(conn, Message{From: "user-agent", To: "message-hub", Content: payload})
}

func sendMsg(conn net.Conn, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[user-agent] marshal error: %v\n", err)
		return
	}
	if _, err := fmt.Fprintf(conn, "%s\n", data); err != nil {
		fmt.Fprintf(os.Stderr, "[user-agent] send error: %v\n", err)
	}
}
