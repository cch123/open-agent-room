package main

import (
	"context"
	"strings"
	"testing"
	"time"
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
			ID:           "agent_lin",
			Name:         "Lin",
			Persona:      "Implementation agent",
			SystemPrompt: "Always return concise implementation notes.",
			Runtime:      "codex",
		},
		Recent: []protocol.Message{
			{AuthorName: "You", Text: strings.Repeat("中文🙂", 260)},
		},
	}

	got := buildRunnerPrompt(request)
	if !utf8.ValidString(got) {
		t.Fatalf("prompt returned invalid UTF-8")
	}
	if !strings.Contains(got, "System prompt:\nAlways return concise implementation notes.") {
		t.Fatalf("prompt missing system prompt:\n%s", got)
	}
}

func TestBuildRunnerPromptIncludesPeerAgentMentions(t *testing.T) {
	request := runnerRequest{
		EventType: "agent.message",
		ChannelID: "chan_general",
		Prompt:    "@Lin @FullstackDev compare options",
		Agent: protocol.Agent{
			ID:   "agent_lin",
			Name: "Lin",
		},
		PeerAgents: []protocol.Agent{
			{
				ID:      "agent_fullstack_dev",
				Name:    "FullstackDev",
				Persona: "Local Claude runtime",
			},
		},
	}

	got := buildRunnerPrompt(request)
	for _, want := range []string{
		"Other agents addressed in the same user message:",
		"@FullstackDev",
		"Collaboration rule:",
		"THREAD_STATUS: continue",
		"THREAD_STATUS: standby",
		"THREAD_STATUS: final",
		"explicitly mention the other participant using the exact @handle listed above",
		"under 8 lines",
		"Use plain chat only during peer discussion",
		"no Markdown headings",
		"final Markdown document between these exact markers",
		"<<<MARKDOWN_DOCUMENT>>>",
		"<<<END_MARKDOWN_DOCUMENT>>>",
		"Any handoff note, caveat, or @You message must be outside the markers",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestParseRouteDecisionNormalizesAgentTargets(t *testing.T) {
	decision, err := parseRouteDecision("```json\n{\"agentIds\":[\"QA\",\"@Architect\",\"missing\"],\"reason\":\"needs review\"}\n```", []protocol.Agent{
		{ID: "agent_qa", Name: "QA"},
		{ID: "agent_architect", Name: "Architect"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(decision.AgentIDs, ","), "agent_qa,agent_architect"; got != want {
		t.Fatalf("agent ids = %s, want %s", got, want)
	}
	if decision.Reason != "needs review" {
		t.Fatalf("reason = %q", decision.Reason)
	}
}

func TestHeuristicRouteDecisionCanChooseNoOne(t *testing.T) {
	decision := heuristicRouteDecision(protocol.RouteArbitrationPayload{
		Channel: protocol.Channel{ID: "chan_general"},
		Message: protocol.Message{ID: "msg_1", Text: "收到"},
		Agents:  []protocol.Agent{{ID: "agent_qa", Name: "QA"}},
	})
	if len(decision.AgentIDs) != 0 {
		t.Fatalf("terminal message should choose no agent, got %v", decision.AgentIDs)
	}
}

func TestBuildRunnerPromptIncludesImportedSkills(t *testing.T) {
	request := runnerRequest{
		EventType: "agent.message",
		ChannelID: "chan_general",
		Prompt:    "review this",
		Agent: protocol.Agent{
			ID:   "agent_lin",
			Name: "Lin",
			Skills: []protocol.AgentSkill{
				{
					ID:      "skill_review",
					Name:    "Review Discipline",
					Source:  "SKILL.md",
					Content: "Lead with defects, cite file and line, keep summary secondary.",
				},
			},
		},
	}

	got := buildRunnerPrompt(request)
	for _, want := range []string{
		"Imported skills for this agent:",
		"## Review Discipline (SKILL.md)",
		"Lead with defects",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildRunnerPromptIncludesTaskWorkflowForAssignedTasks(t *testing.T) {
	request := runnerRequest{
		EventType: "task.assigned",
		ChannelID: "chan_task",
		Workdir:   "/tmp/project",
		Prompt:    "Ship the implementation",
		Agent: protocol.Agent{
			ID:   "agent_owner",
			Name: "Owner",
		},
	}

	got := buildRunnerPrompt(request)
	for _, want := range []string{
		"Task workflow:",
		"Working directory: /tmp/project",
		"Use the working directory above",
		"moved it to Doing",
		"TASK_STATUS: review",
		"Never mark the task Done",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildRunnerPromptIncludesTaskWorkflowForUpdatedTasks(t *testing.T) {
	request := runnerRequest{
		EventType: "task.updated",
		ChannelID: "chan_task",
		Workdir:   "/tmp/project",
		Prompt:    "The acceptance criteria changed",
		Agent: protocol.Agent{
			ID:   "agent_owner",
			Name: "Owner",
		},
	}

	got := buildRunnerPrompt(request)
	for _, want := range []string{
		"Task update workflow:",
		"Working directory: /tmp/project",
		"still the owner",
		"latest source of truth",
		"TASK_STATUS: review",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestTaskRevocationCancelsMatchingActiveRun(t *testing.T) {
	d := &daemon{activeRuns: make(map[string]activeRun)}
	ctx, cancel := context.WithCancel(context.Background())
	d.setActive("agent_owner", "run_1", "chan_task", cancel)

	if !d.cancelAgent("agent_owner", "chan_task") {
		t.Fatal("expected matching active run to be cancelled")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("active run was not cancelled")
	}
}

func TestTaskRevocationDoesNotCancelDifferentChannel(t *testing.T) {
	d := &daemon{activeRuns: make(map[string]activeRun)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.setActive("agent_owner", "run_1", "chan_other", cancel)

	if d.cancelAgent("agent_owner", "chan_task") {
		t.Fatal("different channel should not be cancelled")
	}
	select {
	case <-ctx.Done():
		t.Fatal("run was cancelled for a different channel")
	default:
	}
}

func TestDemoReplyMentionsPeerAgents(t *testing.T) {
	got := buildReply(
		protocol.Agent{ID: "agent_lin", Name: "Lin"},
		"@Lin @FullstackDev compare options",
		nil,
		[]protocol.Agent{{ID: "agent_fullstack_dev", Name: "FullstackDev"}},
	)
	if !strings.Contains(got, "@FullstackDev") {
		t.Fatalf("demo reply should mention peer agent:\n%s", got)
	}
}
