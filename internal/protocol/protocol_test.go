package protocol

import "testing"

func TestExtractMentions(t *testing.T) {
	agents := []Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin Huang"},
		{ID: "agent_architect", Name: "架构师"},
	}

	got := ExtractMentions("@Ada please pair with @Lin%20Huang and @架构师，确认一下", agents)
	if len(got) != 3 {
		t.Fatalf("expected 3 mentions, got %d: %#v", len(got), got)
	}
	if got[0] != "agent_ada" || got[1] != "agent_lin" || got[2] != "agent_architect" {
		t.Fatalf("unexpected mention order: %#v", got)
	}

	got = ExtractMentions("@lin-huang is not a strict match", agents)
	if len(got) != 0 {
		t.Fatalf("hyphenated display name should not match a spaced agent name: %#v", got)
	}
}

func TestMentionHandleEncodesSpaces(t *testing.T) {
	if got := MentionHandle("Fullstack Dev"); got != "@Fullstack%20Dev" {
		t.Fatalf("mention handle = %q", got)
	}
	if got := MentionHandle("架构师"); got != "@架构师" {
		t.Fatalf("mention handle without spaces = %q", got)
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	env := NewEnvelope("srv_test", "agent.status", Actor{Kind: "agent", ID: "agent_ada"}, Scope{Kind: "server", ID: "srv_test"}, AgentStatusPayload{AgentID: "agent_ada", Status: "idle"}, "")

	payload, err := DecodePayload[AgentStatusPayload](env)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Status != "idle" || env.Type != "agent.status" {
		t.Fatalf("bad envelope payload: %#v", payload)
	}
	if env.Trace.CorrelationID == "" {
		t.Fatal("expected correlation id")
	}
}
