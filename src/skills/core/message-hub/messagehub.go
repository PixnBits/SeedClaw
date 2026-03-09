package main

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"os"
)

type Message struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Content string `json:"content"`
}

func main() {
	addr := os.Getenv("CONTROL_ADDR")
	if addr == "" {
		addr = "host.docker.internal:50023"
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Println("Connected to seedclaw at", addr)

	// Send hello
	msg := Message{From: "message-hub", To: "seedclaw", Content: "Hello from message-hub"}
	data, _ := json.Marshal(msg)
	conn.Write(append(data, '\n'))

	// Listen for responses
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			log.Println("Invalid message:", err)
			continue
		}
		log.Printf("Received: %+v\n", msg)
	}
}
