package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xargin/open-agent-room/internal/protocol"
	"github.com/xargin/open-agent-room/internal/realtime"
	"github.com/xargin/open-agent-room/internal/store"
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

func TestPeerAgentsForExcludesCurrentAgent(t *testing.T) {
	agents := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin"},
		{ID: "agent_claudelocal", Name: "ClaudeLocal"},
	}

	got := peerAgentsFor(agents, "agent_lin")
	want := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_claudelocal", Name: "ClaudeLocal"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("peer agents = %v, want %v", got, want)
	}
}

func TestRouteTargetsFromAgentReplyIgnoresHumanAndSelfMentions(t *testing.T) {
	agents := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin"},
		{ID: "agent_claudelocal", Name: "ClaudeLocal"},
	}

	got := routeTargetsFromAgentReply("@ClaudeLocal can you validate this? @You wait for the final", "agent_lin", agents)
	want := []string{"agent_claudelocal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("route targets = %v, want %v", got, want)
	}

	got = routeTargetsFromAgentReply("@Lin I can handle this myself. @You please review.", "agent_lin", agents)
	if len(got) != 0 {
		t.Fatalf("self/human-only mentions should not route, got %v", got)
	}
}

func TestRouteTargetsForReplyPayloadFallsBackToPeersUnlessHandedToHuman(t *testing.T) {
	agents := []protocol.Agent{
		{ID: "agent_lin", Name: "Lin"},
		{ID: "agent_claudelocal", Name: "ClaudeLocal"},
	}

	got := routeTargetsForReplyPayload(protocol.AgentReplyPayload{
		Text:       "I agree with this direction.",
		PeerAgents: []protocol.Agent{{ID: "agent_claudelocal", Name: "ClaudeLocal"}},
	}, "agent_lin", agents)
	if !reflect.DeepEqual(got, []string{"agent_claudelocal"}) {
		t.Fatalf("peer fallback targets = %v", got)
	}

	got = routeTargetsForReplyPayload(protocol.AgentReplyPayload{
		Text:       "@You final summary is ready.",
		PeerAgents: []protocol.Agent{{ID: "agent_claudelocal", Name: "ClaudeLocal"}},
	}, "agent_lin", agents)
	if len(got) != 0 {
		t.Fatalf("human handoff should not route to peers, got %v", got)
	}
}

func TestRouteTargetsForReplyPayloadPrefersExplicitMentions(t *testing.T) {
	agents := []protocol.Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin"},
		{ID: "agent_claudelocal", Name: "ClaudeLocal"},
	}

	got := routeTargetsForReplyPayload(protocol.AgentReplyPayload{
		Text:       "@Ada can you review this?",
		PeerAgents: []protocol.Agent{{ID: "agent_claudelocal", Name: "ClaudeLocal"}},
	}, "agent_lin", agents)
	if !reflect.DeepEqual(got, []string{"agent_ada"}) {
		t.Fatalf("explicit targets = %v", got)
	}
}

func TestMergeAgentsKeepsFirstOccurrenceOrder(t *testing.T) {
	got := mergeAgents(
		[]protocol.Agent{{ID: "agent_lin", Name: "Lin"}, {ID: "agent_ada", Name: "Ada"}},
		[]protocol.Agent{{ID: "agent_lin", Name: "Lin"}, {ID: "agent_claudelocal", Name: "ClaudeLocal"}},
	)
	want := []protocol.Agent{
		{ID: "agent_lin", Name: "Lin"},
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_claudelocal", Name: "ClaudeLocal"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged agents = %v, want %v", got, want)
	}
}

func TestRecentUnhandledAgentMentionRoutesBackfillsAgentReply(t *testing.T) {
	replyEnv := protocol.NewEnvelope("srv_local", "agent.reply", protocol.Actor{Kind: "agent", ID: "agent_claudelocal", Name: "ClaudeLocal"}, protocol.Scope{Kind: "channel", ID: "chan_general"}, protocol.AgentReplyPayload{
		AgentID:     "agent_claudelocal",
		ChannelID:   "chan_general",
		Text:        "@Lin can you validate this?",
		ThreadDepth: 0,
	}, "")
	state := protocol.State{
		Channels: []protocol.Channel{{ID: "chan_general", Name: "general"}},
		Agents: []protocol.Agent{
			{ID: "agent_lin", Name: "Lin"},
			{ID: "agent_claudelocal", Name: "ClaudeLocal"},
		},
		Messages: []protocol.Message{{
			ID:         "msg_1",
			ChannelID:  "chan_general",
			AuthorKind: "agent",
			AuthorID:   "agent_claudelocal",
			AuthorName: "ClaudeLocal",
			Text:       "@Lin can you validate this?",
			ProtocolID: replyEnv.ID,
		}},
		Events: []protocol.Envelope{replyEnv},
	}

	got := recentUnhandledAgentMentionRoutes(state, 30)
	if len(got) != 1 {
		t.Fatalf("routes = %d, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0].TargetIDs, []string{"agent_lin"}) {
		t.Fatalf("targets = %v, want Lin", got[0].TargetIDs)
	}
	if got[0].ThreadDepth != 1 {
		t.Fatalf("thread depth = %d, want 1", got[0].ThreadDepth)
	}
	if !reflect.DeepEqual(got[0].PeerPool, []protocol.Agent{{ID: "agent_claudelocal", Name: "ClaudeLocal"}, {ID: "agent_lin", Name: "Lin"}}) {
		t.Fatalf("peer pool = %v", got[0].PeerPool)
	}
}

func TestRecentUnhandledAgentMentionRoutesSkipsHandledReply(t *testing.T) {
	replyEnv := protocol.NewEnvelope("srv_local", "agent.reply", protocol.Actor{Kind: "agent", ID: "agent_claudelocal", Name: "ClaudeLocal"}, protocol.Scope{Kind: "channel", ID: "chan_general"}, protocol.AgentReplyPayload{
		AgentID:   "agent_claudelocal",
		ChannelID: "chan_general",
		Text:      "@Lin can you validate this?",
	}, "")
	routedEnv := protocol.NewEnvelope("srv_local", "agent.message", protocol.Actor{Kind: "system", ID: "router"}, protocol.Scope{Kind: "channel", ID: "chan_general"}, protocol.AgentMessagePayload{}, replyEnv.ID)
	state := protocol.State{
		Channels: []protocol.Channel{{ID: "chan_general", Name: "general"}},
		Agents: []protocol.Agent{
			{ID: "agent_lin", Name: "Lin"},
			{ID: "agent_claudelocal", Name: "ClaudeLocal"},
		},
		Messages: []protocol.Message{{
			ID:         "msg_1",
			ChannelID:  "chan_general",
			AuthorKind: "agent",
			AuthorID:   "agent_claudelocal",
			Text:       "@Lin can you validate this?",
			ProtocolID: replyEnv.ID,
		}},
		Events: []protocol.Envelope{replyEnv, routedEnv},
	}

	got := recentUnhandledAgentMentionRoutes(state, 30)
	if len(got) != 0 {
		t.Fatalf("handled reply should not backfill, got %v", got)
	}
}

func TestRecentUnhandledAgentMentionRoutesSkipsTrimmedReplyEvent(t *testing.T) {
	state := protocol.State{
		Channels: []protocol.Channel{{ID: "chan_general", Name: "general"}},
		Agents: []protocol.Agent{
			{ID: "agent_lin", Name: "Lin"},
			{ID: "agent_claudelocal", Name: "ClaudeLocal"},
		},
		Messages: []protocol.Message{{
			ID:         "msg_1",
			ChannelID:  "chan_general",
			AuthorKind: "agent",
			AuthorID:   "agent_claudelocal",
			Text:       "@Lin can you validate this?",
			ProtocolID: "evt_trimmed_reply",
		}},
	}

	got := recentUnhandledAgentMentionRoutes(state, 30)
	if len(got) != 0 {
		t.Fatalf("trimmed reply event should not backfill, got %v", got)
	}
}

func TestHandleSkillsFetchesContentFromSourceURL(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Architecture patterns\n\nPrefer small contexts."))
	}))
	defer source.Close()

	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/skills", strings.NewReader(`{"name":"Architecture","source":"`+source.URL+`"}`))
	rec := httptest.NewRecorder()

	a.handleSkills(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var skill protocol.AgentSkill
	if err := json.NewDecoder(rec.Body).Decode(&skill); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skill.Content, "Prefer small contexts.") {
		t.Fatalf("content = %q, want fetched source content", skill.Content)
	}
}

func TestHandleSkillsReportsSourceFetchFailure(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer source.Close()

	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/skills", strings.NewReader(`{"name":"Missing","source":"`+source.URL+`"}`))
	rec := httptest.NewRecorder()

	a.handleSkills(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "HTTP 404") {
		t.Fatalf("body = %s, want HTTP 404 error", rec.Body.String())
	}
}

func newTestApp(t *testing.T) *app {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &app{
		store:        st,
		hub:          realtime.NewHub(),
		daemons:      newDaemonRegistry(),
		token:        "test-token",
		activeAgents: make(map[string]string),
	}
}
