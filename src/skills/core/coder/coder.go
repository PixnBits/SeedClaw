package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
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
	regMsg := Message{From: "coder", To: "message-hub", Content: "register"}
	data, _ := json.Marshal(regMsg)
	conn.Write(append(data, '\n'))

	// Listen for messages
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			log.Println("Invalid message:", err)
			continue
		}
		log.Printf("Received: %+v\n", msg)

		if msg.To == "coder" {
			handleCodeGeneration(msg, conn)
		}
	}
}

func handleCodeGeneration(msg Message, conn net.Conn) {
	content, ok := msg.Content.(map[string]interface{})
	if !ok {
		log.Println("Invalid content format")
		return
	}

	action, ok := content["action"].(string)
	if !ok || action != "generate_skill" {
		log.Println("Unsupported action")
		return
	}

	skillName, ok := content["skill_name"].(string)
	if !ok {
		log.Println("No skill_name specified")
		return
	}

	prompt, ok := content["prompt"].(string)
	if !ok {
		log.Println("No prompt specified")
		return
	}

	// Generate code using the model
	generatedCode, err := generateSkillCode(skillName, prompt)
	if err != nil {
		log.Println("Code generation error:", err)
		reply := Message{
			From: "coder",
			To:   msg.From,
			Content: map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			},
		}
		data, _ := json.Marshal(reply)
		conn.Write(append(data, '\n'))
		return
	}

	reply := Message{
		From: "coder",
		To:   msg.From,
		Content: map[string]interface{}{
			"status": "success",
			"skill":  generatedCode,
		},
	}
	data, _ := json.Marshal(reply)
	conn.Write(append(data, '\n'))
}

func generateSkillCode(skillName, prompt string) (map[string]interface{}, error) {
	// Create a detailed prompt for code generation
	fullPrompt := fmt.Sprintf(`Generate a complete SeedClaw skill package for "%s".

Requirements:
%s

Please provide a complete skill implementation including:
1. SKILL.md - Description and capabilities
2. main.go - Go source code that connects to message-hub
3. Dockerfile - Container build instructions

Format the response as JSON with keys: "skill_md", "go_code", "dockerfile"

Skill name: %s
`, skillName, prompt, skillName)

	reqBody := map[string]interface{}{
		"model":  "qwen2.5-coder:32b",
		"prompt": fullPrompt,
		"stream": false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "http://ollama:11434/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, err
	}

	response, ok := ollamaResp["response"].(string)
	if !ok {
		return nil, fmt.Errorf("no response in reply")
	}

	// Parse the JSON response from the model
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &result); err != nil {
		return nil, fmt.Errorf("failed to parse generated code: %v", err)
	}

	return result, nil
}
