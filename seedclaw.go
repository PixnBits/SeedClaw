package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type SkillConfig struct {
	Name    string
	Image   string
	Mounts  []string
	Cmd     []string
	Network string
}

type IPCRequest struct {
	Action  string            `json:"action"`
	Name    string            `json:"name,omitempty"`
	Image   string            `json:"image,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Mounts  []string          `json:"mounts,omitempty"`
	Cmd     []string          `json:"cmd,omitempty"`
	Network string            `json:"network,omitempty"`
	Lines   int               `json:"lines,omitempty"`
}

var cli *client.Client
var ipcSock = filepath.Join(os.Getenv("HOME"), ".seedclaw", "ipc.sock")

func main() {
	var err error
	cli, err = client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatal(err)
	}

	os.MkdirAll(filepath.Dir(ipcSock), 0700)
	os.Remove(ipcSock)

	go startIPCServer()

	startCoreSkills()

	go stdinChat()

	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		go telegramChat(token)
	}

	select {}
}

func startIPCServer() {
	l, err := net.Listen("unix", ipcSock)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	os.Chmod(ipcSock, 0600)
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go handleIPC(conn)
	}
}

func handleIPC(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var req IPCRequest
		if err := json.Unmarshal([]byte(scanner.Text()), &req); err != nil {
			conn.Write([]byte(`{"error":"invalid json"}` + "\n"))
			continue
		}
		resp := handleAction(req)
		conn.Write([]byte(resp + "\n"))
	}
}

func handleAction(req IPCRequest) string {
	switch req.Action {
	case "start_skill":
		return startSkill(req.Name, req.Image, req.Env, req.Mounts, req.Cmd, req.Network)
	case "stop_skill":
		return stopSkill(req.Name)
	case "restart_skill":
		stopSkill(req.Name)
		return startSkill(req.Name, req.Image, req.Env, req.Mounts, req.Cmd, req.Network)
	case "get_logs":
		return getLogs(req.Name, req.Lines)
	case "get_status":
		return getStatus(req.Name)
	default:
		return `{"error":"unknown action"}`
	}
}

func startSkill(name, image string, env map[string]string, mounts []string, cmd []string, network string) string {
	ctx := context.Background()
	envSlice := []string{}
	for k, v := range env {
		envSlice = append(envSlice, k+"="+v)
	}
	mountSlice := []mount.Mount{}
	for _, m := range mounts {
		parts := strings.Split(m, ":")
		if len(parts) >= 2 {
			mountSlice = append(mountSlice, mount.Mount{
				Type:     mount.TypeBind,
				Source:   parts[0],
				Target:   parts[1],
				ReadOnly: len(parts) > 2 && parts[2] == "ro",
			})
		}
	}
	config := &container.Config{
		Image:     image,
		Env:       envSlice,
		Cmd:       cmd,
		OpenStdin: true,
	}
	hostConfig := &container.HostConfig{
		Mounts:      mountSlice,
		NetworkMode: container.NetworkMode(network),
	}
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err != nil {
		log.Printf("ContainerCreate error for %s: %v", name, err)
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Printf("ContainerStart error for %s: %v", name, err)
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	go readLogs(name)
	return `{"status":"started"}`
}

func stopSkill(name string) string {
	ctx := context.Background()
	timeoutSec := 10
	if err := cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeoutSec}); err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	return `{"status":"stopped"}`
}

func getLogs(name string, lines int) string {
	ctx := context.Background()
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", lines),
	}
	reader, err := cli.ContainerLogs(ctx, name, options)
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	defer reader.Close()
	logs, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	return fmt.Sprintf(`{"logs":"%s"}`, string(logs))
}

func getStatus(name string) string {
	ctx := context.Background()
	inspect, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	return fmt.Sprintf(`{"status":"%s"}`, inspect.State.Status)
}

func startCoreSkills() {
	skills := []string{"messagehub", "llmcaller", "coder", "skill-builder"}
	for _, s := range skills {
		var folder string
		if s == "messagehub" || s == "llmcaller" {
			folder = "core"
		} else {
			folder = "sdlc"
		}
		config := parseMD(filepath.Join("skills", folder, s+".md"))
		log.Printf("Parsed config for %s: image=%q, network=%q", config.Name, config.Image, config.Network)
		log.Printf("Starting skill %s with image %s, network %s", config.Name, config.Image, config.Network)
		startSkill(config.Name, config.Image, nil, config.Mounts, config.Cmd, config.Network)
	}
}

func parseMD(path string) SkillConfig {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	config := SkillConfig{Name: strings.TrimSuffix(filepath.Base(path), ".md")}
	inSpec := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "**Docker run spec**") {
			inSpec = true
		}
		if inSpec {
			if strings.HasPrefix(line, "- Image: ") {
				config.Image = strings.TrimSpace(strings.TrimPrefix(line, "- Image: "))
			}
			if strings.HasPrefix(line, "- Mount: ") {
				mountStr := strings.TrimPrefix(line, "- Mount: ")
				mountStr = strings.ReplaceAll(mountStr, "$HOME", os.Getenv("HOME"))
				config.Mounts = strings.Split(mountStr, ", ")
			}
			if strings.HasPrefix(line, "- Command: ") {
				config.Cmd = strings.Split(strings.TrimSpace(strings.TrimPrefix(line, "- Command: ")), " ")
			}
			if strings.HasPrefix(line, "- Network: ") {
				config.Network = strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "- Network: ")))[0]
			}
		}
	}
	return config
}

func readLogs(name string) {
	ctx := context.Background()
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	}
	reader, err := cli.ContainerLogs(ctx, name, options)
	if err != nil {
		log.Println(err)
		return
	}
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(scanner.Text()), &msg); err != nil {
			continue
		}
		if to, ok := msg["to"].(string); ok && to == "user" {
			if payload, ok := msg["payload"].(map[string]interface{}); ok {
				if text, ok := payload["text"].(string); ok {
					fmt.Println(text)
				}
			}
		} else if to != "" {
			sendToSkill(to, msg)
		}
	}
}

func stdinChat() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		message := map[string]interface{}{
			"from":    "user",
			"to":      "messagehub",
			"type":    "text",
			"payload": map[string]string{"text": scanner.Text()},
		}
		sendToSkill("messagehub", message)
	}
}

func sendToSkill(name string, msg interface{}) {
	data, _ := json.Marshal(msg)
	cmd := exec.Command("docker", "exec", "-i", name, "sh", "-c", "cat")
	cmd.Stdin = strings.NewReader(string(data) + "\n")
	cmd.Run()
}

func telegramChat(token string) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Println(err)
		return
	}
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message != nil {
			message := map[string]interface{}{
				"from":    "user",
				"to":      "messagehub",
				"type":    "text",
				"payload": map[string]string{"text": update.Message.Text},
			}
			sendToSkill("messagehub", message)
		}
	}
}
