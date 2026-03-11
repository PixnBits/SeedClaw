package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Message struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Content string `json:"content"`
}

var hubConn net.Conn
var hubMu sync.Mutex

func main() {
	// Create shared dirs
	sharedDir := "./shared"
	subdirs := []string{"sockets/control", "sources", "builds", "outputs", "logs", "audit"}
	for _, sub := range subdirs {
		path := filepath.Join(sharedDir, sub)
		os.MkdirAll(path, 0755)
	}

	// Listen on TCP for message-hub
	listener, err := net.Listen("tcp", ":50023")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	log.Println("Seedclaw listening on :50023")

	// Build and start core skills via docker compose (detached)
	log.Println("Building core skill images...")
	if out, err := exec.Command("docker", "compose", "build").CombinedOutput(); err != nil {
		log.Println("docker compose build failed:", err)
		log.Println(string(out))
	} else {
		log.Println("Build output:", string(out))
	}

	log.Println("Starting core skills (detached)...")
	if out, err := exec.Command("docker", "compose", "up", "-d").CombinedOutput(); err != nil {
		log.Println("docker compose up -d failed:", err)
		log.Println(string(out))
	} else {
		log.Println("Compose up output:", string(out))
	}
	time.Sleep(5 * time.Second)

	// Accept connection from message-hub and keep handle for stdin forwarding
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		log.Println("Message-hub connected")

		hubMu.Lock()
		hubConn = conn
		hubMu.Unlock()

		// Handle messages
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			var msg Message
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				log.Println("Invalid message:", err)
				continue
			}
			log.Printf("Received: %+v\n", msg)
			// Echo back
			response := Message{From: "seedclaw", To: msg.From, Content: "Echo: " + msg.Content}
			data, _ := json.Marshal(response)
			conn.Write(append(data, '\n'))
			log.Printf("Sent: %+v\n", response)
		}

		hubMu.Lock()
		hubConn = nil
		hubMu.Unlock()
		conn.Close()
		log.Println("Message-hub disconnected")
	}()

	// Stdin loop: any line is forwarded to the `llm-caller` which will
	// translate the user request into skill calls and route them accordingly.
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Enter a request and press Enter; it will be translated by the LLM into skill calls:")
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			log.Println("Stdin closed; entering idle mode (seedclaw will keep running).")
			select {}
		}
		input := scanner.Text()
		log.Println("User input:", input)

		// Wait for message-hub connection (bounded) before sending; avoids dropping user input
		start := time.Now()
		var c net.Conn
		for {
			hubMu.Lock()
			c = hubConn
			hubMu.Unlock()
			if c != nil {
				break
			}
			if time.Since(start) > 60*time.Second {
				log.Println("Timeout waiting for message-hub; dropping input")
				c = nil
				break
			}
			log.Println("Waiting for message-hub to connect...")
			time.Sleep(500 * time.Millisecond)
		}
		if c == nil {
			continue
		}

		// Send to llm-caller for translation
		msg := Message{From: "seedclaw", To: "llm-caller", Content: input}
		data, _ := json.Marshal(msg)
		c.Write(append(data, '\n'))
		log.Println("Forwarded input to llm-caller for translation")
	}
}
