package protocol

import "testing"

func TestExtractMentions(t *testing.T) {
	agents := []Agent{
		{ID: "agent_ada", Name: "Ada"},
		{ID: "agent_lin", Name: "Lin Huang"},
	}

	got := ExtractMentions("@Ada please pair with @lin-huang", agents)
	if len(got) != 2 {
		t.Fatalf("expected 2 mentions, got %d: %#v", len(got), got)
	}
	if got[0] != "agent_ada" || got[1] != "agent_lin" {
		t.Fatalf("unexpected mention order: %#v", got)
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
