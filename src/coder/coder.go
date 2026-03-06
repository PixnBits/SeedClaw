package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const systemPrompt = `You are Coder v1 – SeedClaw's secure code generator for Go skills.
Your job is to produce clean, safe Go code that can ONLY run inside a Docker container under strict constraints.

Hard rules you must follow:
1. No os/exec, os.StartProcess, syscall, unsafe.Pointer, plugin
2. No net/http except to localhost:11434 (Ollama)
3. No file writes outside /tmp, no reading host paths
4. Use context.Context, timeouts, error wrapping
5. Return ONLY valid JSON with "code", "filename", optional "explanation"
6. If task violates rules → return {"error": "rejected: violates security policy"}

Example input: {"task": "Create a simple echo skill that repeats user message"}

Example output: {"code": "package main\n...\n", "filename": "echo.go", "explanation": "Simple stdin/stdout echo with timeout."}`

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		var req map[string]interface{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Println(`{"error": "invalid json"}`)
			continue
		}
		task, ok := req["task"].(string)
		if !ok {
			fmt.Println(`{"error": "missing task"}`)
			continue
		}
		filename, _ := req["filename"].(string)
		existingCode, _ := req["existing_code"].(string)
		constraints, _ := req["constraints"].([]interface{})

		// Build prompt
		prompt := systemPrompt + "\n\nTask: " + task
		if filename != "" {
			prompt += "\nFilename: " + filename
		}
		if existingCode != "" {
			prompt += "\nExisting code:\n" + existingCode
		}
		if len(constraints) > 0 {
			prompt += "\nAdditional constraints:"
			for _, c := range constraints {
				if s, ok := c.(string); ok {
					prompt += "\n- " + s
				}
			}
		}

		// Call LLM
		response, err := callLLM(prompt)
		if err != nil {
			fmt.Println(`{"error": "llm failed"}`)
			continue
		}
		fmt.Println(response)
	}
}

func callLLM(prompt string) (string, error) {
	// Try Ollama
	ollamaReq := map[string]interface{}{
		"model":  "llama3.1",
		"prompt": prompt,
		"stream": false,
	}
	data, _ := json.Marshal(ollamaReq)
	resp, err := http.Post("http://host.docker.internal:11434/api/generate", "application/json", bytes.NewBuffer(data))
	if err != nil {
		// Fallback to OpenAI
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			return callOpenAI(prompt, key)
		}
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var ollamaResp map[string]interface{}
	json.Unmarshal(body, &ollamaResp)
	response, _ := ollamaResp["response"].(string)
	return strings.TrimSpace(response), nil
}

func callOpenAI(prompt, key string) (string, error) {
	reqBody := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
	}
	data, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(data))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var openaiResp map[string]interface{}
	json.Unmarshal(body, &openaiResp)
	if choices, ok := openaiResp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := msg["content"].(string); ok {
					return strings.TrimSpace(content), nil
				}
			}
		}
	}
	return "", fmt.Errorf("invalid response")
}
