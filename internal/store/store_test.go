package store

import (
	"path/filepath"
	"testing"
)

func TestDeleteChannelRemovesMessagesAndKeepsOneChannel(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := st.DeleteChannel("chan_general")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != "chan_general" {
		t.Fatalf("deleted channel = %s, want chan_general", deleted.ID)
	}
	for _, msg := range st.Snapshot().Messages {
		if msg.ChannelID == "chan_general" {
			t.Fatalf("message %s for deleted channel remained", msg.ID)
		}
	}
	if _, err := st.DeleteChannel("chan_build-room"); err == nil {
		t.Fatal("expected deleting the last channel to fail")
	}
}

func TestDeleteAgentUpdatesChannelDefaults(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteAgent("agent_ada"); err != nil {
		t.Fatal(err)
	}

	snapshot := st.Snapshot()
	for _, agent := range snapshot.Agents {
		if agent.ID == "agent_ada" {
			t.Fatal("deleted agent remained in snapshot")
		}
	}
	for _, ch := range snapshot.Channels {
		if ch.DefaultAgentID == "agent_ada" {
			t.Fatalf("channel %s still defaults to deleted agent", ch.ID)
		}
		for _, memberID := range ch.MemberIDs {
			if memberID == "agent_ada" {
				t.Fatalf("channel %s still contains deleted agent", ch.ID)
			}
		}
	}
}
