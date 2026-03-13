// messagehub.go – SeedClaw message-hub core skill v2.1
//
// SINGLE SOURCE OF TRUTH: src/skills/core/message-hub/SKILL.md, ARCHITECTURE.md v2.1, PRD.md v2.1
//
// message-hub is the SOLE IPC router for the entire SeedClaw swarm.
//
// SECURITY INVARIANTS:
// 1. This process dials OUT to host.internal:7124. It NEVER listens on any host-exposed port.
// 2. All skill connections arrive inbound on message-hub's container listen port (8765).
// 3. Only registered skills are routed to.
// 4. Audit events are forwarded to seedclaw over the control channel – never written to disk.
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
	"sync"
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

// ─────────────────────────────────────────────
//  Skill registry (in-memory)
// ─────────────────────────────────────────────

var (
	skillsMu sync.RWMutex
	// skills maps skill name → the TCP connection from that skill to us.
	skills = map[string]net.Conn{}
	// seedclawConn is the persistent connection to the seedclaw control plane.
	seedclawConn net.Conn
)

func main() {
	fmt.Fprintln(os.Stderr, "[message-hub] starting …")

	// CONTROL_PLANE_PATTERN_v2.1.3: Client TCP retry – exponential backoff
	// SECURITY_INVARIANT: ARCHITECTURE.md §Communication Architecture –
	//   message-hub is the only skill that connects to host.internal:7124.
	seedclawConn = connectToSeedclaw()
	fmt.Fprintln(os.Stderr, "[message-hub] connected to seedclaw control plane")

	// Announce to seedclaw.
	sendToSeedclaw(Message{
		From:    "message-hub",
		To:      "seedclaw",
		Content: json.RawMessage(`{"action":"hub_ready"}`),
	})

	// Listen for inbound connections from other skills.
	hubPort := os.Getenv("HUB_PORT")
	if hubPort == "" {
		hubPort = "8765"
	}

	// Note: We listen on 0.0.0.0 INSIDE the container. The container is isolated
	// to seedclaw-net and no port is published to the host.
	ln, err := net.Listen("tcp", "0.0.0.0:"+hubPort)
	if err != nil {
		sendAuditEvent("hub_listen_failed", "fatal", err.Error())
		fmt.Fprintf(os.Stderr, "[message-hub] FATAL: cannot listen: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	sendAuditEvent("hub_listen", "ok", "listening on :"+hubPort)
	fmt.Fprintf(os.Stderr, "[message-hub] listening for skills on :%s\n", hubPort)

	// Start reading from the seedclaw control channel in a goroutine.
	// This delivers REPL messages (from user-agent to skills) to their destinations.
	go readFromSeedclaw()

	// Accept skill connections.
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[message-hub] accept error: %v\n", err)
			continue
		}
		go handleSkillConn(conn)
	}
}

// ─────────────────────────────────────────────
//  Seedclaw control plane client
// ─────────────────────────────────────────────

// connectToSeedclaw dials seedclaw's loopback TCP port with exponential back-off.
//
// CONTROL_PLANE_PATTERN_v2.1.3: Client TCP retry – exponential backoff
// SECURITY_INVARIANT: ARCHITECTURE.md § Control Plane Access –
//
//	ONLY message-hub receives host.internal alias.
func connectToSeedclaw() net.Conn {
	controlPort := os.Getenv("SEEDCLAW_CONTROL_PORT")
	if controlPort == "" {
		controlPort = "7124"
	}
	addr := "host.internal:" + controlPort

	var (
		conn net.Conn
		err  error
	)
	for attempt := 0; attempt < 30; attempt++ {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			return conn
		}
		delay := time.Duration(500*(1<<uint(attempt))) * time.Millisecond
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		fmt.Fprintf(os.Stderr, "[message-hub] seedclaw not ready (attempt %d): %v – retry in %s\n", attempt+1, err, delay)
		time.Sleep(delay)
	}
	fmt.Fprintf(os.Stderr, "[message-hub] FATAL: cannot connect to seedclaw: %v\n", err)
	os.Exit(1)
	return nil
}

// readFromSeedclaw processes messages arriving FROM seedclaw (e.g. REPL user input).
//
// CONTROL_PLANE_PATTERN_v2.1.3: Active bufio.Scanner forever loop
// SECURITY_INVARIANT: prevents all-goroutines-asleep deadlock
func readFromSeedclaw() {
	scanner := bufio.NewScanner(seedclawConn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Fprintf(os.Stderr, "[message-hub] bad JSON from seedclaw: %v\n", err)
			continue
		}
		routeMessage(msg)
	}
	fmt.Fprintln(os.Stderr, "[message-hub] seedclaw connection closed")
}

func sendToSeedclaw(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[message-hub] marshal error: %v\n", err)
		return
	}
	if _, err := fmt.Fprintf(seedclawConn, "%s\n", data); err != nil {
		fmt.Fprintf(os.Stderr, "[message-hub] write to seedclaw error: %v\n", err)
	}
}

// sendAuditEvent forwards an audit event to seedclaw for immutable logging.
// SECURITY_INVARIANT: ARCHITECTURE.md §Auditing – message-hub never writes to disk.
func sendAuditEvent(action, status, detail string) {
	payload, _ := json.Marshal(map[string]string{
		"action": "audit_event",
		"event":  action,
		"status": status,
		"detail": detail,
	})
	sendToSeedclaw(Message{
		From:    "message-hub",
		To:      "seedclaw",
		Content: payload,
	})
}

// ─────────────────────────────────────────────
//  Skill connection handler
// ─────────────────────────────────────────────

// handleSkillConn reads messages from a connected skill and routes them.
//
// CONTROL_PLANE_PATTERN_v2.1.3: Active bufio.Scanner forever loop
func handleSkillConn(conn net.Conn) {
	defer conn.Close()

	remote := conn.RemoteAddr().String()
	var skillName string

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Fprintf(os.Stderr, "[message-hub] bad JSON from %s: %v\n", remote, err)
			continue
		}

		// First message must be a registration.
		if skillName == "" {
			var content map[string]string
			if err := json.Unmarshal(msg.Content, &content); err == nil && content["action"] == "register" {
				skillName = content["skill"]
				if skillName == "" {
					skillName = msg.From
				}
				registerSkill(skillName, conn)
				sendAuditEvent("skill_connected", "ok", skillName)
				fmt.Fprintf(os.Stderr, "[message-hub] skill registered: %s\n", skillName)
				continue
			}
			// No explicit registration; fall back to From field.
			skillName = msg.From
			if skillName != "" {
				registerSkill(skillName, conn)
			}
		}

		// Validate sender matches registered name.
		msg.Metadata.SenderValidated = (msg.From == skillName)
		if !msg.Metadata.SenderValidated {
			sendAuditEvent("sender_mismatch", "warn",
				fmt.Sprintf("from=%q registered=%q", msg.From, skillName))
		}

		routeMessage(msg)
	}

	if skillName != "" {
		unregisterSkill(skillName)
		sendAuditEvent("skill_disconnected", "ok", skillName)
	}
}

// ─────────────────────────────────────────────
//  Router
// ─────────────────────────────────────────────

// routeMessage delivers a message to its intended recipient.
// SECURITY_INVARIANT: all inter-skill communication routes through here exclusively.
func routeMessage(msg Message) {
	// Log every routed message as an audit event.
	sendAuditEvent("route_message", "ok",
		fmt.Sprintf("from=%q to=%q", msg.From, msg.To))

	switch msg.To {
	case "seedclaw":
		sendToSeedclaw(msg)
	case "message-hub":
		// Messages addressed to message-hub itself (e.g. audit events from skills).
		// Forward them to seedclaw so they can be logged immutably.
		sendToSeedclaw(msg)
	default:
		skillsMu.RLock()
		dest, ok := skills[msg.To]
		skillsMu.RUnlock()

		if !ok {
			// Destination skill not connected – inform the sender.
			errPayload, _ := json.Marshal(map[string]string{
				"action": "route_error",
				"error":  fmt.Sprintf("skill %q not connected", msg.To),
			})
			errMsg := Message{From: "message-hub", To: msg.From, Content: errPayload}
			data, _ := json.Marshal(errMsg)

			skillsMu.RLock()
			src, srcOk := skills[msg.From]
			skillsMu.RUnlock()
			if srcOk {
				fmt.Fprintf(src, "%s\n", data) //nolint:errcheck
			}
			return
		}

		data, err := json.Marshal(msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[message-hub] marshal error routing to %s: %v\n", msg.To, err)
			return
		}
		if _, err := fmt.Fprintf(dest, "%s\n", data); err != nil {
			fmt.Fprintf(os.Stderr, "[message-hub] delivery error to %s: %v\n", msg.To, err)
			unregisterSkill(msg.To)
		}
	}
}

// ─────────────────────────────────────────────
//  Skill registry helpers
// ─────────────────────────────────────────────

func registerSkill(name string, conn net.Conn) {
	skillsMu.Lock()
	defer skillsMu.Unlock()
	skills[name] = conn
}

func unregisterSkill(name string) {
	skillsMu.Lock()
	defer skillsMu.Unlock()
	delete(skills, name)
}
