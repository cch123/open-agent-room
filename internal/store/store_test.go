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

func TestAddUserJoinsExistingAndNewChannels(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	user, err := st.AddUser("Taylor")
	if err != nil {
		t.Fatal(err)
	}

	snapshot := st.Snapshot()
	for _, ch := range snapshot.Channels {
		if !contains(ch.MemberIDs, user.ID) {
			t.Fatalf("channel %s missing registered human %s", ch.ID, user.ID)
		}
	}

	ch, err := st.AddChannel("planning", "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(ch.MemberIDs, user.ID) {
		t.Fatalf("new channel missing registered human %s", user.ID)
	}
}

func TestDeleteUserRemovesMembershipButKeepsCurrentHuman(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.AddUser("Taylor")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.DeleteUser(user.ID); err != nil {
		t.Fatal(err)
	}
	snapshot := st.Snapshot()
	for _, ch := range snapshot.Channels {
		if contains(ch.MemberIDs, user.ID) {
			t.Fatalf("channel %s still contains deleted human %s", ch.ID, user.ID)
		}
	}
	if _, err := st.DeleteUser("usr_you"); err == nil {
		t.Fatal("expected deleting current human to fail")
	}
}

func TestAddAndDeleteAgentSkill(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	skill, err := st.AddAgentSkill("agent_ada", "Review Discipline", "SKILL.md", "Lead with defects.")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := st.Snapshot()
	var found bool
	for _, agent := range snapshot.Agents {
		if agent.ID == "agent_ada" && len(agent.Skills) == 1 && agent.Skills[0].ID == skill.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("imported skill %s was not stored on agent_ada", skill.ID)
	}

	if _, err := st.DeleteAgentSkill("agent_ada", skill.ID); err != nil {
		t.Fatal(err)
	}
	snapshot = st.Snapshot()
	for _, agent := range snapshot.Agents {
		if agent.ID == "agent_ada" && len(agent.Skills) != 0 {
			t.Fatalf("deleted skill remained on agent_ada: %+v", agent.Skills)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
