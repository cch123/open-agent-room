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
	channel := protocol.Channel{ID: "chan_general", MemberIDs: []string{"usr_you", "agent_ada", "agent_lin"}}

	got := app.resolveAgentRoutes(channel, "@Ada can you review this?", agents)
	want := []string{"agent_ada"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit mention = %v, want %v", got, want)
	}

	got = app.resolveAgentRoutes(channel, "what would you change first?", agents)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("implicit follow-up = %v, want %v", got, want)
	}

	otherChannel := protocol.Channel{ID: "chan_other"}
	got = app.resolveAgentRoutes(otherChannel, "this channel uses the global default agent", agents)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default fallback = %v, want %v", got, want)
	}
}

func TestResolveAgentRoutesClearsAmbiguousMultiAgentContext(t *testing.T) {
	app := &app{activeAgents: make(map[string]string)}
	agents := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin"},
	}
	channel := protocol.Channel{ID: "chan_general", MemberIDs: []string{"usr_you", "agent_ada", "agent_lin"}}

	_ = app.resolveAgentRoutes(channel, "@Ada start this thread", agents)
	got := app.resolveAgentRoutes(channel, "@Ada @Lin compare options", agents)
	want := []string{"agent_ada", "agent_lin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("multi-agent mention = %v, want %v", got, want)
	}

	got = app.resolveAgentRoutes(channel, "who owns the next step?", agents)
	want = []string{"agent_ada"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ambiguous follow-up uses channel default = %v, want %v", got, want)
	}
}

func TestResolveAgentRoutesUsesFirstChannelAgentByDefault(t *testing.T) {
	app := &app{activeAgents: make(map[string]string)}
	agents := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin"},
	}
	channel := protocol.Channel{ID: "chan_build", MemberIDs: []string{"usr_you", "agent_lin", "agent_ada"}}

	got := app.resolveAgentRoutes(channel, "first message in this channel", agents)
	want := []string{"agent_lin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("channel default = %v, want %v", got, want)
	}

	got = app.resolveAgentRoutes(channel, "second message follows the default active agent", agents)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default becomes active = %v, want %v", got, want)
	}
}

func TestResolveAgentRoutesUsesConfiguredChannelDefault(t *testing.T) {
	app := &app{activeAgents: make(map[string]string)}
	agents := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin"},
	}
	channel := protocol.Channel{
		ID:             "chan_general",
		MemberIDs:      []string{"usr_you", "agent_ada", "agent_lin"},
		DefaultAgentID: "agent_lin",
	}

	got := app.resolveAgentRoutes(channel, "first message without mention", agents)
	want := []string{"agent_lin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("configured default = %v, want %v", got, want)
	}
}
