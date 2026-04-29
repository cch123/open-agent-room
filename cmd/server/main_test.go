package main

import (
	"reflect"
	"testing"

	"github.com/xargin/open-agent-room/internal/protocol"
)

func TestResolveAgentRoutesKeepsSingleAgentContext(t *testing.T) {
	app := &app{activeAgents: make(map[string]string)}
	agents := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin"},
	}

	got := app.resolveAgentRoutes("chan_general", "@Ada can you review this?", agents)
	want := []string{"agent_ada"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit mention = %v, want %v", got, want)
	}

	got = app.resolveAgentRoutes("chan_general", "what would you change first?", agents)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("implicit follow-up = %v, want %v", got, want)
	}

	got = app.resolveAgentRoutes("chan_other", "this channel has no active agent", agents)
	if len(got) != 0 {
		t.Fatalf("unrelated channel = %v, want no routes", got)
	}
}

func TestResolveAgentRoutesClearsAmbiguousMultiAgentContext(t *testing.T) {
	app := &app{activeAgents: make(map[string]string)}
	agents := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin"},
	}

	_ = app.resolveAgentRoutes("chan_general", "@Ada start this thread", agents)
	got := app.resolveAgentRoutes("chan_general", "@Ada @Lin compare options", agents)
	want := []string{"agent_ada", "agent_lin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("multi-agent mention = %v, want %v", got, want)
	}

	got = app.resolveAgentRoutes("chan_general", "who owns the next step?", agents)
	if len(got) != 0 {
		t.Fatalf("ambiguous follow-up = %v, want no routes", got)
	}
}
