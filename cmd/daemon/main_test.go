package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xargin/open-agent-room/internal/protocol"
)

func TestCompactKeepsUTF8ValidWhenTruncatingMultibyteText(t *testing.T) {
	text := strings.Repeat("设计", 300)
	got := compact(text, 500)
	if !utf8.ValidString(got) {
		t.Fatalf("compact returned invalid UTF-8: %q", got)
	}
	if len([]rune(got)) > 500 {
		t.Fatalf("compact returned %d runes, want at most 500", len([]rune(got)))
	}
}

func TestBuildRunnerPromptKeepsUTF8ValidForRecentContext(t *testing.T) {
	request := runnerRequest{
		EventType: "agent.message",
		ChannelID: "chan_general",
		Prompt:    strings.Repeat("继续", 20),
		Agent: protocol.Agent{
			ID:      "agent_lin",
			Name:    "Lin",
			Persona: "Implementation agent",
			Runtime: "codex",
		},
		Recent: []protocol.Message{
			{AuthorName: "You", Text: strings.Repeat("中文🙂", 260)},
		},
	}

	got := buildRunnerPrompt(request)
	if !utf8.ValidString(got) {
		t.Fatalf("prompt returned invalid UTF-8")
	}
}

func TestBuildRunnerPromptIncludesPeerAgentMentions(t *testing.T) {
	request := runnerRequest{
		EventType: "agent.message",
		ChannelID: "chan_general",
		Prompt:    "@Lin @ClaudeLocal compare options",
		Agent: protocol.Agent{
			ID:   "agent_lin",
			Name: "Lin",
		},
		PeerAgents: []protocol.Agent{
			{
				ID:      "agent_claudelocal",
				Name:    "ClaudeLocal",
				Persona: "Local Claude runtime",
			},
		},
	}

	got := buildRunnerPrompt(request)
	for _, want := range []string{
		"Other agents addressed in the same user message:",
		"@ClaudeLocal",
		"Collaboration rule:",
		"explicitly mention the other participant with @Name",
		"mention @You with a concise final summary",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestDemoReplyMentionsPeerAgents(t *testing.T) {
	got := buildReply(
		protocol.Agent{ID: "agent_lin", Name: "Lin"},
		"@Lin @ClaudeLocal compare options",
		nil,
		[]protocol.Agent{{ID: "agent_claudelocal", Name: "ClaudeLocal"}},
	)
	if !strings.Contains(got, "@ClaudeLocal") {
		t.Fatalf("demo reply should mention peer agent:\n%s", got)
	}
}
