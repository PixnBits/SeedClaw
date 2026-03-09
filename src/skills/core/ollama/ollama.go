package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
)

type Message struct {
	From    string      `json:"from"`
	To      string      `json:"to"`
	Content interface{} `json:"content"`
}

func main() {
	addr := os.Getenv("CONTROL_ADDR")
	if addr == "" {
		addr = "message-hub:50024"
	}

	// Connect to message-hub
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Println("Connected to message-hub at", addr)

	// Send register
	regMsg := Message{From: "ollama", To: "message-hub", Content: "register"}
	data, _ := json.Marshal(regMsg)
	conn.Write(append(data, '\n'))

	// Start Ollama serve in background
	go func() {
		cmd := exec.Command("ollama", "serve")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Println("Ollama serve error:", err)
		}
	}()

	// Listen for messages
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			log.Println("Invalid message:", err)
			continue
		}
		log.Printf("Received: %+v\n", msg)

		if msg.To == "ollama" {
			handleMessage(msg, conn)
		}
	}
}

func handleMessage(msg Message, conn net.Conn) {
	content, ok := msg.Content.(map[string]interface{})
	if !ok {
		log.Println("Invalid content format")
		return
	}

	action, ok := content["action"].(string)
	if !ok {
		log.Println("No action specified")
		return
	}

	var response interface{}
	var err error

	switch action {
	case "pull":
		model, ok := content["model"].(string)
		if !ok {
			err = fmt.Errorf("no model specified")
		} else {
			response, err = pullModel(model)
		}
	case "list":
		response, err = listModels()
	default:
		err = fmt.Errorf("unknown action: %s", action)
	}

	status := "success"
	if err != nil {
		status = "error"
		response = err.Error()
	}

	reply := Message{
		From: "ollama",
		To:   msg.From,
		Content: map[string]interface{}{
			"status": status,
			"result": response,
		},
	}
	data, _ := json.Marshal(reply)
	conn.Write(append(data, '\n'))
}

func pullModel(model string) (string, error) {
	cmd := exec.Command("ollama", "pull", model)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pull failed: %s", string(output))
	}
	return fmt.Sprintf("Pulled model %s", model), nil
}

func listModels() ([]string, error) {
	cmd := exec.Command("ollama", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	var models []string
	for _, line := range lines[1:] { // Skip header
		if strings.TrimSpace(line) != "" {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				models = append(models, fields[0])
			}
		}
	}
	return models, nil
}
