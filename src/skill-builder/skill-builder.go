package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
		code, ok := req["code"].(string)
		if !ok {
			fmt.Println(`{"error": "missing code"}`)
			continue
		}
		filename, _ := req["filename"].(string)
		if filename == "" {
			filename = "skill.go"
		}
		skillName, _ := req["skill_name"].(string)
		if skillName == "" {
			fmt.Println(`{"error": "missing skill_name"}`)
			continue
		}
		dockerBase, _ := req["docker_base"].(string)
		if dockerBase == "" {
			dockerBase = "alpine:latest"
		}

		// Write code to /tmp/filename
		filePath := filepath.Join("/tmp", filename)
		if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
			fmt.Println(`{"error": "failed to write code"}`)
			continue
		}

		// Build
		binPath := "/tmp/skillbin"
		cmd := exec.Command("go", "build", "-o", binPath, filePath)
		if err := cmd.Run(); err != nil {
			fmt.Println(`{"error": "build failed"}`)
			continue
		}

		// Vet
		cmd = exec.Command("go", "vet", filePath)
		if err := cmd.Run(); err != nil {
			fmt.Println(`{"error": "vet failed"}`)
			continue
		}

		// Test: run with test input
		testInput := `{"test": "input"}`
		cmd = exec.Command(binPath)
		cmd.Stdin = strings.NewReader(testInput + "\n")
		output, err := cmd.Output()
		if err != nil {
			fmt.Println(`{"error": "test failed"}`)
			continue
		}
		var testResp map[string]interface{}
		if err := json.Unmarshal(output, &testResp); err != nil {
			fmt.Println(`{"error": "test output not json"}`)
			continue
		}

		// Construct manifest
		manifest := map[string]interface{}{
			"name":   skillName,
			"image":  "seedclaw-" + skillName + ":v1",
			"env":    map[string]string{},
			"mounts": []string{"/ipc.sock:/ipc.sock:ro"},
			"cmd":    []string{"/app/skillbin"},
		}

		// Send IPC
		conn, err := net.Dial("unix", "/ipc.sock")
		if err != nil {
			fmt.Println(`{"error": "ipc connect failed"}`)
			continue
		}
		defer conn.Close()
		ipcReq := map[string]interface{}{
			"action": "start_skill",
			"name":   skillName,
			"image":  manifest["image"].(string),
			"env":    manifest["env"].(map[string]string),
			"mounts": manifest["mounts"].([]string),
			"cmd":    manifest["cmd"].([]string),
		}
		data, _ := json.Marshal(ipcReq)
		conn.Write(append(data, '\n'))

		// Output status
		fmt.Printf(`{"status": "requested", "manifest": %s}`, string(data))
	}
}
