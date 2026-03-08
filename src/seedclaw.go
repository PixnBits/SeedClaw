package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"time"
)

const (
	socketPath  = "/tmp/seedclaw.sock"
	composeFile = "compose.yaml"
)

func main() {
	start := flag.Bool("start", false, "Start core services and enter chat mode")
	flag.Parse()

	if *start {
		if err := startCoreServices(); err != nil {
			log.Fatalf("Failed to start core services: %v", err)
		}
	}

	if err := runChat(); err != nil {
		log.Fatalf("Chat loop failed: %v", err)
	}
}

func startCoreServices() error {
	// Clean up old socket
	_ = os.Remove(socketPath)

	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		return fmt.Errorf("compose.yaml not found")
	}

	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}

	// Give container a moment to start
	time.Sleep(1500 * time.Millisecond)

	fmt.Println("Core services started. Unix socket ready:", socketPath)
	return nil
}

func runChat() error {
	fmt.Println("SeedClaw chat mode. Type messages below (empty line or Ctrl+D to exit)")

	// Create listener on host
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("cannot listen on unix socket: %w", err)
	}
	defer ln.Close()

	// Make sure everyone can connect (especially important if container runs non-root)
	if err := os.Chmod(socketPath, 0666); err != nil {
		log.Printf("Warning: chmod failed: %v", err)
	}

	// Accept exactly one connection (MVP — seedclaw ↔ message-hub)
	fmt.Println("Waiting for message-hub to connect...")
	conn, err := ln.Accept()
	if err != nil {
		return fmt.Errorf("accept failed: %w", err)
	}
	fmt.Println("Message-hub connected!")

	// Forward replies from hub → stdout in background
	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Println("[hub]", scanner.Text())
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			log.Printf("read from hub failed: %v", err)
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}

		_, err := fmt.Fprintf(conn, "%s\n", line)
		if err != nil {
			return fmt.Errorf("write to hub failed: %w", err)
		}
	}

	fmt.Println("Exiting chat.")
	conn.Close()
	return nil
}
