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
	From    string `json:"from"`
	To      string `json:"to"`
	Content string `json:"content"`
}

// For translation results we may parse messages back
type OutMessage struct {
	From    string      `json:"from"`
	To      string      `json:"to"`
	Content interface{} `json:"content"`
}

type OpenAIRequest struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
}

type OpenAIChoice struct {
	Message OpenAIMessage `json:"message"`
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
	regMsg := Message{From: "llm-caller", To: "message-hub", Content: "register"}
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

		if msg.To == "llm-caller" {
			// Treat content as a user prompt to be translated into skill calls
			translated, err := translateToSkillCalls(msg.Content)
			if err != nil {
				log.Println("Translate error:", err)
				// fallback: return plain LLM text
				response, err2 := callLLM(msg.Content)
				if err2 != nil {
					response = "Error: " + err.Error()
				}
				reply := Message{From: "llm-caller", To: msg.From, Content: response}
				data, _ := json.Marshal(reply)
				conn.Write(append(data, '\n'))
				continue
			}

			// Send each produced skill message via the message-hub (conn)
			for _, out := range translated {
				b, _ := json.Marshal(out)
				conn.Write(append(b, '\n'))
			}

			// Inform original requester
			summary := fmt.Sprintf("Issued %d skill calls", len(translated))
			reply := Message{From: "llm-caller", To: msg.From, Content: summary}
			data, _ := json.Marshal(reply)
			conn.Write(append(data, '\n'))
		}
	}
}

func callLLM(prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model":  "llama3.2:latest",
		"prompt": prompt,
		"stream": false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "http://ollama:11434/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", err
	}

	response, ok := ollamaResp["response"].(string)
	if !ok {
		return "", fmt.Errorf("no response in reply")
	}

	return strings.TrimSpace(response), nil
}

// translateToSkillCalls asks the LLM to convert a user prompt into
// a JSON array of messages to send to skills (format: OutMessage).
func translateToSkillCalls(prompt string) ([]OutMessage, error) {
	system := `You are an assistant that converts a user's high-level request into a sequence of SeedClaw skill messages.
Output must be a JSON array of objects with keys: from, to, content. Content may be a string or an object.
Example:
[{"from":"llm-caller","to":"coder","content":{"action":"generate_skill","skill_name":"hello-world","prompt":"..."}}]
Only output the JSON array.`

	full := system + "\nUser request:\n" + prompt
	resp, err := callLLM(full)
	if err != nil {
		return nil, err
	}

	var arr []OutMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp)), &arr); err != nil {
		return nil, fmt.Errorf("failed to parse LLM JSON output: %v; raw: %s", err, resp)
	}
	return arr, nil
}
