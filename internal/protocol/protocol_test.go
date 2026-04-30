package protocol

import "testing"

func TestExtractMentions(t *testing.T) {
	agents := []Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "LinHuang"},
		{ID: "agent_architect", Name: "架构师"},
	}

	got := ExtractMentions("@Ada please pair with @LinHuang and @架构师，确认一下", agents)
	if len(got) != 3 {
		t.Fatalf("expected 3 mentions, got %d: %#v", len(got), got)
	}
	if got[0] != "agent_ada" || got[1] != "agent_lin" || got[2] != "agent_architect" {
		t.Fatalf("unexpected mention order: %#v", got)
	}

	got = ExtractMentions("@lin-huang is not a strict match", agents)
	if len(got) != 0 {
		t.Fatalf("spaced raw mention should not match an agent name: %#v", got)
	}
}

func TestMentionHandleForAgentNames(t *testing.T) {
	if got := MentionHandle("FullstackDev"); got != "@FullstackDev" {
		t.Fatalf("mention handle = %q", got)
	}
	if got := MentionHandle("架构师"); got != "@架构师" {
		t.Fatalf("mention handle without spaces = %q", got)
	}
}

func TestStripThreadStatusMarker(t *testing.T) {
	clean, status := StripThreadStatusMarker("@QA 收到，保持待命。\nTHREAD_STATUS: standby")
	if clean != "@QA 收到，保持待命。" {
		t.Fatalf("clean text = %q", clean)
	}
	if status != ThreadStatusStandby {
		t.Fatalf("thread status = %q", status)
	}
	if !ThreadStatusStopsRouting(status) {
		t.Fatal("standby should stop routing")
	}

	clean, status = StripThreadStatusMarker("Need QA input.\nTHREAD_STATUS: needs_response")
	if clean != "Need QA input." || status != ThreadStatusContinue {
		t.Fatalf("continue marker parsed as clean=%q status=%q", clean, status)
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
