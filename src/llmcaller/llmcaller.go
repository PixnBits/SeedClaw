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

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		var req map[string]interface{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Println(`{"error": "invalid json"}`)
			continue
		}
		prompt, ok := req["prompt"].(string)
		if !ok {
			fmt.Println(`{"error": "missing prompt"}`)
			continue
		}
		// Basic sanitize
		if strings.Contains(strings.ToLower(prompt), "ignore") ||
			strings.Contains(strings.ToLower(prompt), "system") ||
			strings.Contains(strings.ToLower(prompt), "override") {
			fmt.Println(`{"error": "sanitized"}`)
			continue
		}
		model := "llama3.1"
		if m, ok := req["model"].(string); ok {
			model = m
		}
		// Call Ollama
		ollamaReq := map[string]interface{}{
			"model":  model,
			"prompt": prompt,
			"stream": false,
		}
		if temp, ok := req["temperature"].(float64); ok {
			ollamaReq["temperature"] = temp
		}
		data, _ := json.Marshal(ollamaReq)
		resp, err := http.Post("http://host.docker.internal:11434/api/generate", "application/json", bytes.NewBuffer(data))
		if err != nil {
			// Fallback to OpenAI if configured
			if key := os.Getenv("OPENAI_API_KEY"); key != "" {
				openaiResp, err := callOpenAI(prompt, model, key)
				if err != nil {
					fmt.Println(`{"error": "llm failed"}`)
					continue
				}
				fmt.Println(openaiResp)
			} else {
				fmt.Println(`{"error": "ollama failed"}`)
			}
			continue
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var ollamaResp map[string]interface{}
		json.Unmarshal(body, &ollamaResp)
		response, _ := ollamaResp["response"].(string)
		fmt.Printf(`{"response": "%s", "finish_reason": "stop", "usage": {"prompt_tokens": 0, "completion_tokens": 0}}`+"\n", strings.ReplaceAll(response, "\n", "\\n"))
	}
}

func callOpenAI(prompt, model, key string) (string, error) {
	// Simplified OpenAI call
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
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
					return fmt.Sprintf(`{"response": "%s", "finish_reason": "stop", "usage": {"prompt_tokens": 0, "completion_tokens": 0}}`, strings.ReplaceAll(content, "\n", "\\n")), nil
				}
			}
		}
	}
	return "", fmt.Errorf("invalid response")
}
