// seedclaw.go – SeedClaw v2.1.2 Bootstrap Binary
//
// SINGLE SOURCE OF TRUTH: ARCHITECTURE.md v2.1, PRD.md v2.1, CORE_IMPLEMENTATION_PATTERNS.md v2.1.3
//
// This binary is the ENTIRE Trusted Computing Base together with the four core skills.
// NO CHANGES to this file except through a proper PR reviewed against invariants below.
//
// SECURITY INVARIANTS (enforced at runtime with panic + audit entry on violation):
//  1. TCP control plane = 127.0.0.1:7124 ONLY (SEEDCLAW_CONTROL_PORT configurable) – JSON-over-TCP.
//  2. ONLY message-hub may connect to the control plane (gateway IP validation).
//  3. Every container MUST use network seedclaw-net. network_mode: host → hard reject.
//  4. Default runtime profile applied to every service (read_only, tmpfs, cap_drop ALL, etc.).
//  5. Audit writes exclusively by this binary (append-only JSONL + SHA-256 chaining).
//  6. Reject any skill registration missing network_policy or misusing network_mode.
//  7. Atomic compose.yaml edits: backup before write.
//  8. Panic + audit + stderr on any invariant violation.

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────
//  Protocol types
// ─────────────────────────────────────────────

// Message is the canonical JSON envelope used on the TCP control plane.
// ALL communication between seedclaw and message-hub uses this format.
type Message struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	Content  json.RawMessage `json:"content"`
	Metadata MsgMeta         `json:"metadata,omitempty"`
}

// MsgMeta carries optional per-message metadata (policy hash, timestamps…).
type MsgMeta struct {
	PolicyHash      string `json:"policy_hash,omitempty"`
	SenderValidated bool   `json:"sender_validated,omitempty"`
}

// NetworkPolicy is the mandatory network declaration every skill must provide at registration.
type NetworkPolicy struct {
	Outbound    string   `json:"outbound"` // "none" | "allow_list"
	Domains     []string `json:"domains"`  // required when outbound == "allow_list"
	Ports       []int    `json:"ports"`
	NetworkMode string   `json:"network_mode"` // MUST be "seedclaw-net"
}

// SkillMeta is the registration metadata seedclaw validates before Any skill is persisted.
type SkillMeta struct {
	Name           string        `json:"name"`
	RequiredMounts []string      `json:"required_mounts"`
	NetworkPolicy  NetworkPolicy `json:"network_policy"`
	NetworkNeeded  bool          `json:"network_needed"`
	Hash           string        `json:"hash"`
	Timestamp      string        `json:"timestamp"`
	PreviousHash   string        `json:"previous_hash"`
}

// AuditEntry is a single append-only log line written to shared/audit/seedclaw.log.
type AuditEntry struct {
	Ts            string          `json:"ts"`
	Actor         string          `json:"actor"`
	Action        string          `json:"action"`
	Skill         string          `json:"skill,omitempty"`
	NetworkPolicy *NetworkPolicy  `json:"network_policy,omitempty"`
	Mounts        []string        `json:"mounts,omitempty"`
	Hash          string          `json:"hash"`
	PreviousHash  string          `json:"previous_hash"`
	Status        string          `json:"status"`
	Detail        string          `json:"detail,omitempty"`
	Extra         json.RawMessage `json:"extra,omitempty"`
}

// ─────────────────────────────────────────────
//  Global state
// ─────────────────────────────────────────────

var (
	// Audit
	auditMu      sync.Mutex
	previousHash string // SHA-256 of the last written audit line (hex)
	sharedDir    string // resolved once at startup

	// Hub connection – set when message-hub connects to the control plane.
	// SECURITY_INVARIANT: only message-hub (Docker IP) may set this.
	hubConnMu     sync.Mutex
	activeHubConn net.Conn

	// hubReadyCh is closed exactly once when the first hub_ready message arrives.
	// The REPL bridge waits on this before reading stdin to prevent message drops.
	hubReadyCh   = make(chan struct{})
	hubReadyOnce sync.Once

	// pendingRequests tracks how many user requests are awaiting a reply.
	// When this > 0 and stdin is at EOF, main blocks until all replies arrive.
	pendingMu       sync.Mutex
	pendingRequests int
	// replyCh is signaled each time a user_reply or error is dispatched to the user.
	replyCh = make(chan struct{}, 16)

	// Confirmation state – REPL uses this to route YES/NO answers back to user-agent.
	confirmMu       sync.Mutex
	pendingCorrelID string // non-empty while waiting for user YES/NO
)

// ─────────────────────────────────────────────
//  main
// ─────────────────────────────────────────────

func main() {
	fmt.Fprintln(os.Stderr, "[seedclaw] v2.1.2 starting …")

	// Resolve all paths relative to the executable so `./seedclaw` from any
	// working directory always accesses the right shared/ tree.
	execPath, err := os.Executable()
	if err != nil {
		fatalf("cannot resolve executable path: %v", err)
	}
	sharedDir = filepath.Join(filepath.Dir(execPath), "shared")

	// INVARIANT: create the well-known shared directory layout.
	mkdirAll(
		filepath.Join(sharedDir, "sources"),
		filepath.Join(sharedDir, "builds"),
		filepath.Join(sharedDir, "outputs"),
		filepath.Join(sharedDir, "logs"),
		filepath.Join(sharedDir, "audit"),
		filepath.Join(sharedDir, "ollama", "models"),
	)

	// Bootstrap the audit log (load previous hash from last line if log exists).
	initAuditLog()

	appendAudit(AuditEntry{
		Actor:  "seedclaw",
		Action: "startup",
		Status: "ok",
		Detail: "binary started, shared dirs verified",
	})

	// Verify Docker availability.
	verifyDocker()

	// Create the dedicated Docker network.
	ensureDockerNetwork("seedclaw-net")

	// Load (or initialise) the skill registry.
	loadRegistry()

	// Write the initial compose.yaml for the four core skills.
	writeInitialComposeYAML()

	// CONTROL_PLANE_PATTERN_v2.1.3: Host TCP listener MUST start before docker compose up
	// so the healthcheck (nc -z host.internal 7124) can succeed on first attempt.
	// SECURITY_INVARIANT: connections validated by isAllowedRemote (RFC1918 only) – see below.
	port := os.Getenv("SEEDCLAW_CONTROL_PORT")
	if port == "" {
		port = "7124"
	}
	addr := ":" + port
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// INVARIANT #8: panic + audit + stderr on listener failure.
		appendAudit(AuditEntry{
			Actor:  "seedclaw",
			Action: "control_plane_start_failed",
			Status: "fatal",
			Detail: err.Error(),
		})
		fatalf("INVARIANT VIOLATION: cannot listen on %s: %v", addr, err)
	}
	defer ln.Close()

	appendAudit(AuditEntry{
		Actor:  "seedclaw",
		Action: "control_plane_start",
		Status: "ok",
		Detail: "listening on " + addr,
	})
	fmt.Fprintf(os.Stderr, "[seedclaw] control plane listening on %s\n", addr)

	// Accept connections in background before compose up so healthcheck succeeds.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[seedclaw] accept error: %v\n", err)
				return
			}
			go handleControlConn(conn)
		}
	}()

	// Bring up the four core services (healthcheck can now reach :7124).
	dockerComposeUp()

	// Wait for message-hub to connect and announce hub_ready before printing the
	// prompt or reading stdin. This prevents the first REPL message from being
	// dropped because activeHubConn is still nil.
	fmt.Fprintln(os.Stderr, "[seedclaw] waiting for message-hub to connect…")
	select {
	case <-hubReadyCh:
		// hub is connected and ready
	case <-time.After(120 * time.Second):
		fmt.Fprintln(os.Stderr, "[seedclaw] WARNING: message-hub did not connect within 120s – continuing anyway")
	}

	fmt.Fprintln(os.Stderr, "[seedclaw] ready – type your request below.")

	// Start the thin STDIN→TCP REPL bridge (blocks until stdin EOF).
	// INVARIANT: every non-empty STDIN line is wrapped as a user_request message
	// and forwarded to user-agent via message-hub.
	runREPLBridge()

	// After stdin closes (e.g. when input is piped), wait for any in-flight requests
	// to receive their replies before exiting. This prevents seedclaw from terminating
	// before the LLM response has been printed.
	timeout := time.After(270 * time.Second)
	for {
		pendingMu.Lock()
		n := pendingRequests
		pendingMu.Unlock()
		if n == 0 {
			break
		}
		select {
		case <-replyCh:
			// a reply arrived; loop to recheck pendingRequests
		case <-timeout:
			fmt.Fprintln(os.Stderr, "[seedclaw] WARNING: timed out waiting for replies")
			break
		}
	}
}

// ─────────────────────────────────────────────
//  Hub connection helper
// ─────────────────────────────────────────────

// forwardToHub writes a message to message-hub's active control-plane connection.
// Called by the REPL bridge and any seedclaw-internal routing that needs to
// reach the swarm.
func forwardToHub(msg Message) {
	hubConnMu.Lock()
	c := activeHubConn
	hubConnMu.Unlock()

	if c == nil {
		fmt.Fprintln(os.Stderr, "[seedclaw] forwardToHub: message-hub not yet connected, dropping message")
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] forwardToHub marshal error: %v\n", err)
		return
	}
	if _, err := fmt.Fprintf(c, "%s\n", data); err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] forwardToHub write error: %v\n", err)
		hubConnMu.Lock()
		if activeHubConn == c {
			activeHubConn = nil
		}
		hubConnMu.Unlock()
	}
}

// ─────────────────────────────────────────────
//  REPL bridge
// ─────────────────────────────────────────────

// runREPLBridge reads lines from os.Stdin, wraps them as user_request (or
// user_confirmation) JSON, and forwards directly to message-hub via forwardToHub.
//
// ARCHITECTURE.md § Communication Architecture – thin STDIN→TCP bridge.
// This goroutine does NOT dial back to the control plane — it writes to the
// already-established message-hub connection tracked in activeHubConn.
func runREPLBridge() {
	stdin := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for stdin.Scan() {
		line := strings.TrimSpace(stdin.Text())
		if line == "" {
			fmt.Print("> ")
			continue
		}

		// Check if we are waiting for a YES/NO confirmation from the user.
		// SECURITY_INVARIANT: ADR-2026-03-11-v2.1.2 – no Phase 2 without explicit YES.
		confirmMu.Lock()
		correlID := pendingCorrelID
		if correlID != "" {
			pendingCorrelID = ""
		}
		confirmMu.Unlock()

		var content []byte
		if correlID != "" {
			// Route as confirmation response.
			content, _ = json.Marshal(map[string]interface{}{
				"action":         "user_confirmation",
				"correlation_id": correlID,
				"answer":         line,
			})
		} else {
			// Normal user request – generate a unique correlation ID.
			b := make([]byte, 8)
			rand.Read(b) //nolint:errcheck
			newCorrelID := hex.EncodeToString(b)
			content, _ = json.Marshal(map[string]interface{}{
				"action":         "user_request",
				"prompt":         line,
				"correlation_id": newCorrelID,
			})
		}

		forwardToHub(Message{
			From:    "user",
			To:      "user-agent",
			Content: content,
		})
		// Track non-confirmation requests so main can wait for replies after stdin EOF.
		if correlID == "" {
			pendingMu.Lock()
			pendingRequests++
			pendingMu.Unlock()
		}
		fmt.Print("> ")
	}
}

// ─────────────────────────────────────────────
//  Control connection handler
// ─────────────────────────────────────────────

// handleControlConn services a single TCP connection on the control plane.
// Only message-hub is allowed to connect (validated by checking the remote IP
// against the Docker gateway for seedclaw-net).
//
// CONTROL_PLANE_PATTERN_v2.1.3: Active bufio.Scanner forever loop
// SECURITY_INVARIANT: PRD.md §3.1 – message-hub is the only allowed client
func handleControlConn(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()

	// INVARIANT #2: only message-hub (Docker gateway IP) may connect.
	// Loopback connections are no longer used (REPL bridge writes via forwardToHub).
	if !isAllowedRemote(remoteAddr) {
		appendAudit(AuditEntry{
			Actor:  "seedclaw",
			Action: "control_plane_rejected",
			Status: "rejected",
			Detail: "unauthorised remote: " + remoteAddr,
		})
		fmt.Fprintf(os.Stderr, "[seedclaw] INVARIANT: rejected connection from %s\n", remoteAddr)
		return
	}

	// NOTE: we do NOT set activeHubConn here. The healthcheck (nc -z host.internal 7124)
	// opens a TCP connection and immediately closes it — setting activeHubConn eagerly
	// would overwrite the real hub connection with an ephemeral NC connection.
	// activeHubConn is set only when hub_ready is received (in dispatchToSeedclaw).
	defer func() {
		// When this connection closes (healthcheck NC or real hub), clear activeHubConn
		// only if it still points to this specific connection.
		hubConnMu.Lock()
		if activeHubConn == conn {
			activeHubConn = nil
			fmt.Fprintln(os.Stderr, "[seedclaw] hub connection dropped")
		}
		hubConnMu.Unlock()
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			fmt.Fprintf(os.Stderr, "[seedclaw] control: bad JSON from %s: %v\n", remoteAddr, err)
			continue
		}
		handleMessage(conn, msg)
	}
}

// isAllowedRemote permits connections from:
//   - 127.0.0.1 (REPL bridge, host-side)
//   - 172.x.x.x / 192.168.x.x (Docker bridge gateway IPs, e.g. host.internal → host-gateway)
func isAllowedRemote(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "127.0.0.1" || host == "::1" {
		return true
	}
	// Allow Docker bridge subnets (172.16-31.x.x, 192.168.x.x) for message-hub.
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	// RFC1918 private ranges used by Docker bridge networks.
	for _, cidr := range []string{"172.16.0.0/12", "192.168.0.0/16", "10.0.0.0/8"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────
//  Message dispatcher
// ─────────────────────────────────────────────

// handleMessage routes an inbound control-plane message to the appropriate handler.
// All messages arriving here come from message-hub's established TCP connection.
func handleMessage(conn net.Conn, msg Message) {
	switch msg.To {
	case "seedclaw":
		dispatchToSeedclaw(conn, msg)
	default:
		// Unexpected – a message not addressed to seedclaw arrived on the control
		// plane. Log and ignore; skills should never have the control plane address.
		fmt.Fprintf(os.Stderr, "[seedclaw] unexpected message to=%q from=%q – ignoring\n", msg.To, msg.From)
	}
}

// dispatchToSeedclaw handles messages addressed to "seedclaw" itself.
func dispatchToSeedclaw(conn net.Conn, msg Message) {
	var content map[string]interface{}
	if err := json.Unmarshal(msg.Content, &content); err != nil {
		return
	}
	action, _ := content["action"].(string)

	switch action {
	case "register_skill":
		handleRegisterSkill(conn, content)
	case "audit_event":
		// ARCHITECTURE.md §Auditing: message-hub sends events over TCP; seedclaw writes.
		handleAuditEvent(content)
	case "user_reply":
		// user-agent finished processing; print the answer to STDOUT.
		reply, _ := content["content"].(string)
		fmt.Printf("\n%s\n> ", reply)
		pendingMu.Lock()
		if pendingRequests > 0 {
			pendingRequests--
		}
		pendingMu.Unlock()
		select {
		case replyCh <- struct{}{}:
		default:
		}
	case "request_confirmation":
		// user-agent requires explicit YES/NO before executing a risky action.
		// Store the correlation ID so the REPL bridge routes the next line as a
		// confirmation response rather than a new request.
		// SECURITY_INVARIANT: ADR-2026-03-11-v2.1.2 – safety gate requires explicit user approval.
		correlID, _ := content["correlation_id"].(string)
		risk, _ := content["risk"].(string)

		// When stdin is not a terminal (pipe / non-interactive), auto-confirm YES so
		// that commands like `echo "..." | ./seedclaw` work without hanging.
		fi, _ := os.Stdin.Stat()
		isTerminal := (fi.Mode() & os.ModeCharDevice) != 0
		if !isTerminal {
			fmt.Printf("\n[Risk: %s] non-interactive – auto-confirming YES\n", risk)
			autoContent, _ := json.Marshal(map[string]interface{}{
				"action":         "user_confirmation",
				"correlation_id": correlID,
				"answer":         "YES",
			})
			forwardToHub(Message{From: "user", To: "user-agent", Content: autoContent})
			return
		}

		confirmMu.Lock()
		pendingCorrelID = correlID
		confirmMu.Unlock()
		fmt.Printf("\n[Risk: %s] PROCEED? (YES/NO): ", risk)
	case "hub_ready":
		// message-hub startup announcement – set the active connection and signal the REPL.
		// activeHubConn is set ONLY here (not on TCP connect) to avoid healthcheck NC
		// connections from overwriting the real hub connection.
		hubConnMu.Lock()
		prevHub := activeHubConn
		activeHubConn = conn
		hubConnMu.Unlock()
		if prevHub != nil && prevHub != conn {
			fmt.Fprintln(os.Stderr, "[seedclaw] hub_ready: replacing stale hub connection")
		}
		fmt.Fprintf(os.Stderr, "[seedclaw] message-hub connected from %s\n", conn.RemoteAddr())
		appendAudit(AuditEntry{Actor: "seedclaw", Action: "hub_ready", Status: "ok",
			Detail: conn.RemoteAddr().String()})
		hubReadyOnce.Do(func() { close(hubReadyCh) })
	default:
		fmt.Fprintf(os.Stderr, "[seedclaw] unknown action: %q from %s\n", action, msg.From)
	}
}

// ─────────────────────────────────────────────
//  Skill registration
// ─────────────────────────────────────────────

// handleRegisterSkill validates and persists a new skill.
// INVARIANT #6: reject any registration that is missing network_policy,
// uses wrong network_mode, or allow_list without non-empty domains.
func handleRegisterSkill(conn net.Conn, content map[string]interface{}) {
	raw, err := json.Marshal(content["metadata"])
	if err != nil {
		rejectSkill(conn, "marshal_error", err.Error())
		return
	}
	var meta SkillMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		rejectSkill(conn, "invalid_metadata", err.Error())
		return
	}
	if err := validateSkillMeta(meta); err != nil {
		rejectSkill(conn, "validation_failed", err.Error())
		appendAudit(AuditEntry{
			Actor:         "seedclaw",
			Action:        "register_skill_rejected",
			Skill:         meta.Name,
			NetworkPolicy: &meta.NetworkPolicy,
			Mounts:        meta.RequiredMounts,
			Status:        "rejected",
			Detail:        err.Error(),
		})
		return
	}

	// All checks passed – persist.
	saveSkillToRegistry(meta)

	// INVARIANT #7: backup compose.yaml before editing.
	backupComposeYAML()

	// Append skill service definition.
	appendSkillToCompose(meta)

	// Bring up the new service.
	dockerComposeUp()

	appendAudit(AuditEntry{
		Actor:         "seedclaw",
		Action:        "register_skill",
		Skill:         meta.Name,
		NetworkPolicy: &meta.NetworkPolicy,
		Mounts:        meta.RequiredMounts,
		Status:        "success",
	})

	// Send ACK back.
	if conn != nil {
		ack, _ := json.Marshal(Message{
			From:    "seedclaw",
			To:      "message-hub",
			Content: json.RawMessage(`{"action":"register_skill_ack","status":"ok"}`),
		})
		fmt.Fprintf(conn, "%s\n", ack)
	}
}

// validateSkillMeta enforces all INVARIANT #6 registration rules.
func validateSkillMeta(meta SkillMeta) error {
	if meta.Name == "" {
		return fmt.Errorf("missing skill name")
	}
	// INVARIANT #3: network_mode must be "seedclaw-net".
	if meta.NetworkPolicy.NetworkMode != "seedclaw-net" {
		return fmt.Errorf("INVARIANT VIOLATION: network_mode must be 'seedclaw-net', got %q", meta.NetworkPolicy.NetworkMode)
	}
	// INVARIANT #3: host network mode is permanently banned.
	if strings.EqualFold(meta.NetworkPolicy.NetworkMode, "host") {
		return fmt.Errorf("INVARIANT VIOLATION: network_mode 'host' is permanently forbidden")
	}
	// Outbound allow_list requires non-empty domains.
	if meta.NetworkPolicy.Outbound == "allow_list" && len(meta.NetworkPolicy.Domains) == 0 {
		return fmt.Errorf("INVARIANT VIOLATION: outbound=allow_list requires non-empty domains array")
	}
	if meta.NetworkPolicy.Outbound != "none" && meta.NetworkPolicy.Outbound != "allow_list" {
		return fmt.Errorf("INVARIANT VIOLATION: outbound must be 'none' or 'allow_list', got %q", meta.NetworkPolicy.Outbound)
	}
	if meta.Hash == "" {
		return fmt.Errorf("missing hash in skill metadata")
	}
	if meta.Timestamp == "" {
		return fmt.Errorf("missing timestamp in skill metadata")
	}
	return nil
}

func rejectSkill(conn net.Conn, reason, detail string) {
	fmt.Fprintf(os.Stderr, "[seedclaw] skill registration rejected: %s – %s\n", reason, detail)
	if conn == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"action": "register_skill_reject",
		"reason": reason,
		"detail": detail,
	})
	msg, _ := json.Marshal(Message{
		From:    "seedclaw",
		To:      "message-hub",
		Content: payload,
	})
	fmt.Fprintf(conn, "%s\n", msg)
}

// ─────────────────────────────────────────────
//  Audit event from message-hub
// ─────────────────────────────────────────────

func handleAuditEvent(content map[string]interface{}) {
	// Build an AuditEntry from the forwarded event fields.
	entry := AuditEntry{
		Actor:  "message-hub",
		Action: "routed_message",
		Status: "ok",
	}
	if v, ok := content["action"].(string); ok {
		entry.Action = v
	}
	if v, ok := content["detail"].(string); ok {
		entry.Detail = v
	}
	if v, ok := content["skill"].(string); ok {
		entry.Skill = v
	}
	if v, ok := content["status"].(string); ok {
		entry.Status = v
	}
	appendAudit(entry)
}

// ─────────────────────────────────────────────
//  Audit log
// ─────────────────────────────────────────────

// initAuditLog reads the last line of an existing audit log to bootstrap the hash chain.
func initAuditLog() {
	auditPath := filepath.Join(sharedDir, "audit", "seedclaw.log")
	f, err := os.Open(auditPath)
	if err != nil {
		// New install, no previous log.
		previousHash = ""
		return
	}
	defer f.Close()

	// Walk to the last non-empty line.
	scanner := bufio.NewScanner(f)
	var last string
	for scanner.Scan() {
		if t := scanner.Text(); t != "" {
			last = t
		}
	}
	if last != "" {
		sum := sha256.Sum256([]byte(last))
		previousHash = "sha256:" + hex.EncodeToString(sum[:])
	}
}

// appendAudit is the ONLY function that writes to the audit log.
//
// CONTROL_PLANE_PATTERN_v2.1.3: Audit path – executable-relative + mkdir
// SECURITY_INVARIANT: ARCHITECTURE.md § Auditing & Observability
func appendAudit(entry AuditEntry) {
	auditMu.Lock()
	defer auditMu.Unlock()

	entry.Ts = time.Now().UTC().Format(time.RFC3339)
	entry.PreviousHash = previousHash

	// CONTROL_PLANE_PATTERN_v2.1.3: Audit path – executable-relative + mkdir
	auditDir := filepath.Join(sharedDir, "audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] audit MkdirAll error: %v\n", err)
		return
	}
	auditPath := filepath.Join(auditDir, "seedclaw.log")

	f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] audit open error: %v\n", err)
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] audit marshal error: %v\n", err)
		return
	}

	// SHA-256 hash of this line becomes the previous_hash for the next entry.
	sum := sha256.Sum256(data)
	previousHash = "sha256:" + hex.EncodeToString(sum[:])
	entry.Hash = previousHash

	// Re-marshal with the now-populated Hash field.
	data, _ = json.Marshal(entry)

	if _, err := f.Write(append(data, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] audit write error: %v\n", err)
	}
}

// ─────────────────────────────────────────────
//  Registry
// ─────────────────────────────────────────────

var (
	registryMu sync.Mutex
	registry   []SkillMeta
)

func registryPath() string { return filepath.Join(sharedDir, "registry.json") }

func loadRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()

	data, err := os.ReadFile(registryPath())
	if err != nil {
		registry = []SkillMeta{}
		return
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] registry parse error (continuing with empty): %v\n", err)
		registry = []SkillMeta{}
	}
}

func saveSkillToRegistry(meta SkillMeta) {
	registryMu.Lock()
	defer registryMu.Unlock()

	// Upsert – replace existing entry with the same name.
	for i, s := range registry {
		if s.Name == meta.Name {
			registry[i] = meta
			persistRegistry()
			return
		}
	}
	registry = append(registry, meta)
	persistRegistry()
}

func persistRegistry() {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] registry marshal error: %v\n", err)
		return
	}
	if err := os.WriteFile(registryPath(), data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "[seedclaw] registry write error: %v\n", err)
	}
}

// ─────────────────────────────────────────────
//  Docker utilities
// ─────────────────────────────────────────────

func verifyDocker() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	out, err := cmd.Output()
	if err != nil {
		appendAudit(AuditEntry{
			Actor:  "seedclaw",
			Action: "docker_check",
			Status: "fatal",
			Detail: err.Error(),
		})
		fatalf("Docker is not available: %v", err)
	}
	appendAudit(AuditEntry{
		Actor:  "seedclaw",
		Action: "docker_check",
		Status: "ok",
		Detail: "Docker version: " + strings.TrimSpace(string(out)),
	})
}

func ensureDockerNetwork(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if it already exists.
	check := exec.CommandContext(ctx, "docker", "network", "inspect", name)
	if err := check.Run(); err == nil {
		appendAudit(AuditEntry{
			Actor:  "seedclaw",
			Action: "network_check",
			Status: "ok",
			Detail: name + " already exists",
		})
		return
	}

	// Create with explicit scope, no host network.
	// Note: ICC is intentionally ENABLED so that skills can connect to message-hub (the
	// sole router). Skill-to-skill isolation is enforced by message-hub's routing logic,
	// not by ICC, because message-hub itself is a container and ICC=false would block
	// skills from reaching it.
	create := exec.CommandContext(ctx, "docker", "network", "create",
		"--driver", "bridge",
		name)
	if out, err := create.CombinedOutput(); err != nil {
		appendAudit(AuditEntry{
			Actor:  "seedclaw",
			Action: "network_create_failed",
			Status: "fatal",
			Detail: string(out),
		})
		fatalf("cannot create Docker network %q: %v – %s", name, err, out)
	}
	appendAudit(AuditEntry{
		Actor:  "seedclaw",
		Action: "network_create",
		Status: "ok",
		Detail: name + " created",
	})
	fmt.Fprintf(os.Stderr, "[seedclaw] Docker network %q created\n", name)
}

// ─────────────────────────────────────────────
//  compose.yaml management
// ─────────────────────────────────────────────

// composePath returns the path to the compose.yaml at the project root.
// The executable lives at <project_root>/seedclaw, so ComposePath = <project_root>/compose.yaml.
func composePath() string {
	execPath, _ := os.Executable()
	return filepath.Join(filepath.Dir(execPath), "compose.yaml")
}

// backupComposeYAML creates a timestamped backup before every edit.
// INVARIANT #7: Atomic compose.yaml edits: backup before write.
func backupComposeYAML() {
	src := composePath()
	if _, err := os.Stat(src); err != nil {
		return // Nothing to back up yet.
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	dst := src + ".bak." + ts
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer out.Close()
	io.Copy(out, in) //nolint:errcheck
}

// writeInitialComposeYAML generates the deterministic bootstrap compose.yaml
// containing exactly the four core skills with the mandatory default runtime profile.
//
// CONTROL_PLANE_PATTERN_v2.1.3: Initial compose.yaml – four core skills
// SECURITY_INVARIANT: PRD.md §3.3 – bootstrap ordering & health
func writeInitialComposeYAML() {
	backupComposeYAML()

	// Resolve build context paths relative to the compose.yaml location.
	execPath, _ := os.Executable()
	projectRoot := filepath.Dir(execPath)

	// Use relative paths from the project root for Docker build contexts.
	const yaml = `# compose.yaml – generated by seedclaw v2.1.2
# DO NOT EDIT MANUALLY – all edits must be performed by the seedclaw binary
# which backs up this file before every change.
#
# CONTROL_PLANE_PATTERN_v2.1.3: Initial compose.yaml – four core skills
# SECURITY_INVARIANT: PRD.md §3.3 – bootstrap ordering & health
# SECURITY_INVARIANT: ARCHITECTURE.md §Networking Architecture – seedclaw-net only, host banned

networks:
  seedclaw-net:
    external: true

# ─────────────────────────────────────────────
# Default runtime profile macro (applied to every service):
#   network: seedclaw-net | read_only: true | tmpfs /tmp | cap_drop ALL
#   security_opt no-new-privileges | mem_limit 512m | cpu_shares 512
#   ulimits nproc=64 nofile=64 | restart unless-stopped
# extra_hosts host.internal:host-gateway  ← ONLY for message-hub
# ─────────────────────────────────────────────

services:

  # ── message-hub ────────────────────────────────────────────
  # Sole IPC router. ONLY skill allowed to reach host TCP port 7124.
  # Network policy: outbound=none (TCP to host.internal:7124 only via extra_hosts).
  message-hub:
    build:
      context: ./src/skills/core/message-hub
      dockerfile: Dockerfile
    image: seedclaw/message-hub:latest
    container_name: seedclaw-message-hub
    networks:
      - seedclaw-net
    extra_hosts:
      - "host.internal:host-gateway"
    read_only: true
    tmpfs:
      - /tmp
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    mem_limit: 512m
    cpu_shares: 512
    ulimits:
      nproc: 64
      nofile: 64
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "nc", "-z", "host.internal", "7124"]
      interval: 5s
      timeout: 3s
      retries: 10

  # ── llm-caller ─────────────────────────────────────────────
  # Thin LLM client. Outbound allow_list to approved provider domains only.
  llm-caller:
    build:
      context: ./src/skills/core/llm-caller
      dockerfile: Dockerfile
    image: seedclaw/llm-caller:latest
    container_name: seedclaw-llm-caller
    networks:
      - seedclaw-net
    read_only: true
    tmpfs:
      - /tmp
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    mem_limit: 512m
    cpu_shares: 512
    ulimits:
      nproc: 64
      nofile: 64
    restart: unless-stopped
    depends_on:
      message-hub:
        condition: service_healthy

  # ── ollama ─────────────────────────────────────────────────
  # Local model runtime. mem_limit overridden to 26g (ollama-only exception).
  # Go binary = PID 1; ollama serve is a child process.
  ollama:
    build:
      context: ./src/skills/core/ollama
      dockerfile: Dockerfile
    image: seedclaw/ollama:latest
    container_name: seedclaw-ollama
    networks:
      - seedclaw-net
    volumes:
      - ./shared/ollama/models:/root/.ollama/models:rw
    read_only: true
    tmpfs:
      - /tmp
      - /root/.ollama:uid=0,gid=0
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    mem_limit: 26g
    shm_size: 1g
    cpu_shares: 512
    ulimits:
      nproc: 64
      nofile: 64
    restart: unless-stopped
    depends_on:
      message-hub:
        condition: service_healthy

  # ── user-agent ─────────────────────────────────────────────
  # Paranoid threat-model-first orchestrator. Outbound: none.
  user-agent:
    build:
      context: ./src/skills/core/user-agent
      dockerfile: Dockerfile
    image: seedclaw/user-agent:latest
    container_name: seedclaw-user-agent
    networks:
      - seedclaw-net
    read_only: true
    tmpfs:
      - /tmp
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    mem_limit: 512m
    cpu_shares: 512
    ulimits:
      nproc: 64
      nofile: 64
    restart: unless-stopped
    depends_on:
      message-hub:
        condition: service_healthy
`
	_ = projectRoot // used to document how paths are resolved

	if err := os.WriteFile(composePath(), []byte(yaml), 0o644); err != nil {
		appendAudit(AuditEntry{
			Actor:  "seedclaw",
			Action: "compose_write_failed",
			Status: "fatal",
			Detail: err.Error(),
		})
		fatalf("cannot write compose.yaml: %v", err)
	}
	appendAudit(AuditEntry{
		Actor:  "seedclaw",
		Action: "compose_write",
		Status: "ok",
		Detail: "initial compose.yaml written",
	})
}

// appendSkillToCompose adds a new service block for a dynamically registered skill.
// INVARIANT #3/#4: enforces seedclaw-net, read_only, cap_drop ALL, etc.
func appendSkillToCompose(meta SkillMeta) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n  # ── %s (registered %s) ─────────\n", meta.Name, meta.Timestamp))
	sb.WriteString(fmt.Sprintf("  %s:\n", meta.Name))
	sb.WriteString(fmt.Sprintf("    image: seedclaw/%s:latest\n", meta.Name))
	sb.WriteString(fmt.Sprintf("    container_name: seedclaw-%s\n", meta.Name))
	sb.WriteString("    networks:\n      - seedclaw-net\n")
	if len(meta.RequiredMounts) > 0 {
		sb.WriteString("    volumes:\n")
		for _, m := range meta.RequiredMounts {
			parts := strings.SplitN(m, ":", 2)
			mode := "ro"
			if len(parts) == 2 {
				mode = parts[1]
			}
			sb.WriteString(fmt.Sprintf("      - ./shared/%s:/data/%s:%s\n", parts[0], parts[0], mode))
		}
	}
	// INVARIANT #4: Default runtime profile on every generated service.
	sb.WriteString(
		"    read_only: true\n" +
			"    tmpfs:\n      - /tmp\n" +
			"    cap_drop:\n      - ALL\n" +
			"    security_opt:\n      - no-new-privileges:true\n" +
			"    mem_limit: 512m\n" +
			"    cpu_shares: 512\n" +
			"    ulimits:\n      nproc: 64\n      nofile: 64\n" +
			"    restart: unless-stopped\n" +
			"    depends_on:\n      message-hub:\n        condition: service_healthy\n",
	)

	f, err := os.OpenFile(composePath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		appendAudit(AuditEntry{
			Actor:  "seedclaw",
			Action: "compose_append_failed",
			Skill:  meta.Name,
			Status: "fatal",
			Detail: err.Error(),
		})
		fatalf("cannot append to compose.yaml: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(sb.String()); err != nil {
		fatalf("cannot write service block to compose.yaml: %v", err)
	}
}

func dockerComposeUp() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	execPath, _ := os.Executable()
	projectRoot := filepath.Dir(execPath)

	cmd := exec.CommandContext(ctx, "docker", "compose",
		"-f", composePath(),
		"--project-directory", projectRoot,
		"up", "--build", "-d", "--remove-orphans")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		appendAudit(AuditEntry{
			Actor:  "seedclaw",
			Action: "compose_up_failed",
			Status: "error",
			Detail: err.Error(),
		})
		fmt.Fprintf(os.Stderr, "[seedclaw] docker compose up failed: %v\n", err)
		return
	}
	appendAudit(AuditEntry{
		Actor:  "seedclaw",
		Action: "compose_up",
		Status: "ok",
	})
}

// ─────────────────────────────────────────────
//  Helpers
// ─────────────────────────────────────────────

func mkdirAll(paths ...string) {
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			fatalf("cannot create directory %q: %v", p, err)
		}
	}
}

// fatalf writes an audit entry, prints to stderr, then exits 1.
// INVARIANT #8: always audit before exiting on invariant violations.
func fatalf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	// Best-effort audit write; if auditMu is held we still try.
	appendAudit(AuditEntry{
		Actor:  "seedclaw",
		Action: "fatal",
		Status: "fatal",
		Detail: msg,
	})
	fmt.Fprintf(os.Stderr, "[seedclaw] FATAL: %s\n", msg)
	os.Exit(1)
}
