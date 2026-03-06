package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

var knownSkills = []string{"llmcaller", "coder", "skill-builder"}

func main() {
	log.Println("Messagehub started")
	log.Println(`{"to":"user","payload":{"text":"Messagehub started"}}`)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			fmt.Println(`{"error": "invalid json"}`)
			continue
		}
		if _, ok := msg["from"]; !ok {
			fmt.Println(`{"error": "missing from"}`)
			continue
		}
		to, ok := msg["to"].(string)
		if !ok || to == "" {
			fmt.Println(`{"error": "invalid to"}`)
			continue
		}
		if to == "broadcast" {
			// Send to all known skills + user
			for _, skill := range knownSkills {
				msg["to"] = skill
				data, _ := json.Marshal(msg)
				fmt.Println(string(data))
			}
			msg["to"] = "user"
			data, _ := json.Marshal(msg)
			fmt.Println(string(data))
		} else {
			// Check if known
			isKnown := to == "user"
			for _, skill := range knownSkills {
				if to == skill {
					isKnown = true
					break
				}
			}
			if !isKnown {
				fmt.Println(`{"error": "unknown destination"}`)
				continue
			}
			// Forward as-is
			fmt.Println(line)
		}
	}
}
