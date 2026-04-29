package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type request struct {
	EventType string `json:"eventType"`
	ChannelID string `json:"channelId"`
	Prompt    string `json:"prompt"`
	Agent     struct {
		Name    string `json:"name"`
		Persona string `json:"persona"`
	} `json:"agent"`
	Memories []string `json:"memories"`
	Recent   []struct {
		AuthorName string `json:"authorName"`
		Text       string `json:"text"`
	} `json:"recent"`
}

func main() {
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fmt.Printf("runner could not decode request: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s handled this through the external runner.\n\n", req.Agent.Name)
	fmt.Printf("Event: %s\n", req.EventType)
	fmt.Printf("Channel: %s\n", req.ChannelID)
	fmt.Printf("Task: %s\n", strings.TrimSpace(req.Prompt))
	if req.Agent.Persona != "" {
		fmt.Printf("\nPersona: %s\n", req.Agent.Persona)
	}
	if len(req.Memories) > 0 {
		fmt.Printf("\nMemory: %s\n", req.Memories[len(req.Memories)-1])
	}
	if len(req.Recent) > 0 {
		latest := req.Recent[len(req.Recent)-1]
		fmt.Printf("\nRecent context: %s said %q\n", latest.AuthorName, latest.Text)
	}
}
