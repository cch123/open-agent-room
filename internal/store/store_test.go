package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/xargin/open-agent-room/internal/protocol"
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

func TestAddAgentRejectsSpaces(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.AddAgent("Fullstack Dev", "Builds features", "", "codex", "", nil, nil); err == nil {
		t.Fatal("expected agent names with spaces to fail")
	}
}

func TestLoadNormalizesExistingAgentNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := protocol.State{
		Meta: protocol.Meta{ServerID: "srv_local"},
		Agents: []protocol.Agent{
			{ID: "agent_fullstack_dev", Name: "Fullstack Dev"},
			{ID: "agent_fullstack", Name: "FullstackDev"},
		},
	}
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	agents := st.Snapshot().Agents
	if got := agents[0].Name; got != "FullstackDev" {
		t.Fatalf("first normalized agent name = %q", got)
	}
	if got := agents[1].Name; got != "FullstackDev2" {
		t.Fatalf("duplicate normalized agent name = %q", got)
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

	skill, err := st.AddAgentSkill("agent_ada", "Review Discipline", "SKILL.md", "Lead with defects.", nil)
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

func TestGlobalSkillCanAttachToMultipleAgents(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	skill, err := st.AddSkill("Review Discipline", "SKILL.md", "Lead with defects.", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AttachAgentSkill("agent_ada", skill.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AttachAgentSkill("agent_lin", skill.ID); err != nil {
		t.Fatal(err)
	}

	snapshot := st.Snapshot()
	if len(snapshot.Skills) != 1 {
		t.Fatalf("global skills = %d, want 1", len(snapshot.Skills))
	}
	for _, agentID := range []string{"agent_ada", "agent_lin"} {
		var found bool
		for _, agent := range snapshot.Agents {
			if agent.ID == agentID && len(agent.Skills) == 1 && agent.Skills[0].ID == skill.ID {
				found = true
			}
		}
		if !found {
			t.Fatalf("skill %s was not attached to %s", skill.ID, agentID)
		}
	}
}

func TestDeleteGlobalSkillDetachesFromAgents(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	skill, err := st.AddSkill("Review Discipline", "SKILL.md", "Lead with defects.", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AttachAgentSkill("agent_ada", skill.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteSkill(skill.ID); err != nil {
		t.Fatal(err)
	}

	snapshot := st.Snapshot()
	if len(snapshot.Skills) != 0 {
		t.Fatalf("global skill remained after delete: %+v", snapshot.Skills)
	}
	for _, agent := range snapshot.Agents {
		if len(agent.Skills) != 0 {
			t.Fatalf("deleted global skill remained attached to %s: %+v", agent.ID, agent.Skills)
		}
	}
}

func TestAddSkillNormalizesTags(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	skill, err := st.AddSkill("Review Discipline", "SKILL.md", "Lead with defects.", []string{" Review ", "#Go Workflow", "review", " "})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"review", "go-workflow"}
	if len(skill.Tags) != len(want) {
		t.Fatalf("tags = %v, want %v", skill.Tags, want)
	}
	for i := range want {
		if skill.Tags[i] != want[i] {
			t.Fatalf("tags = %v, want %v", skill.Tags, want)
		}
	}
}

func TestAddAgentStoresSystemPromptAndInitialSkills(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	agent, err := st.AddAgent("Reviewer", "Reviews changes", "Only report concrete findings.", "codex", "default", []protocol.AgentSkill{
		{Name: "Review Discipline", Source: "create-agent", Content: "Lead with defects."},
		{Name: "Go Workflow", Source: "create-agent", Content: "Run go test ./..."},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if agent.SystemPrompt != "Only report concrete findings." {
		t.Fatalf("system prompt = %q", agent.SystemPrompt)
	}
	if len(agent.Skills) != 2 {
		t.Fatalf("skills = %d, want 2", len(agent.Skills))
	}
	for _, skill := range agent.Skills {
		if skill.ID == "" || skill.CreatedAt == "" {
			t.Fatalf("skill was not normalized: %+v", skill)
		}
	}
}

func TestAddAgentAttachesExistingSkillIDs(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	skill, err := st.AddSkill("Review Discipline", "SKILL.md", "Lead with defects.", []string{"review"})
	if err != nil {
		t.Fatal(err)
	}

	agent, err := st.AddAgent("Reviewer", "Reviews changes", "", "codex", "", nil, []string{skill.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(agent.Skills) != 1 || agent.Skills[0].ID != skill.ID {
		t.Fatalf("agent skills = %+v, want attached %s", agent.Skills, skill.ID)
	}
	if len(st.Snapshot().Skills) != 1 {
		t.Fatalf("existing skill should not be duplicated")
	}
}

func TestTasksCanMoveAndOpenDiscussionChannel(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Snapshot().TaskLanes) == 0 {
		t.Fatal("default task lanes were not initialized")
	}

	task, err := st.AddTask("Review routing", "Check mention routing edge cases.", "/tmp/repo", "lane_todo", "usr_you")
	if err != nil {
		t.Fatal(err)
	}
	if task.Workdir != "/tmp/repo" {
		t.Fatalf("task workdir = %q", task.Workdir)
	}
	nextWorkdir := "/tmp/other"
	task, err = st.UpdateTask(task.ID, nil, nil, &nextWorkdir, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if task.Workdir != nextWorkdir {
		t.Fatalf("updated task workdir = %q", task.Workdir)
	}
	doing := "lane_doing"
	updated, err := st.UpdateTask(task.ID, nil, nil, nil, &doing, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LaneID != doing {
		t.Fatalf("task lane = %s, want %s", updated.LaneID, doing)
	}

	linked, ch, created, err := st.CreateTaskChannel(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected first task channel call to create a channel")
	}
	if linked.ChannelID != ch.ID {
		t.Fatalf("task channel = %s, want %s", linked.ChannelID, ch.ID)
	}

	again, same, created, err := st.CreateTaskChannel(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected second task channel call to reuse existing channel")
	}
	if again.ChannelID != linked.ChannelID || same.ID != ch.ID {
		t.Fatalf("task channel was not reused: task=%s channel=%s", again.ChannelID, same.ID)
	}
}

func TestTasksCanBeAssignedToParticipants(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	agent, err := st.AddAgent("Owner", "Owns tasks", "", "codex", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.AddTask("Ship task owners", "", "", "lane_todo", "usr_you")
	if err != nil {
		t.Fatal(err)
	}

	kind, id := "agent", agent.ID
	updated, err := st.UpdateTask(task.ID, nil, nil, nil, nil, &kind, &id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AssigneeKind != "agent" || updated.AssigneeID != agent.ID {
		t.Fatalf("assignee = %s/%s, want agent/%s", updated.AssigneeKind, updated.AssigneeID, agent.ID)
	}

	linked, ch, _, err := st.CreateTaskChannel(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assigned, changed, err := st.AssignTaskByChannel(ch.ID, "human", "usr_you")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected channel assignment to change task owner")
	}
	if assigned.ID != linked.ID || assigned.AssigneeKind != "human" || assigned.AssigneeID != "usr_you" {
		t.Fatalf("assigned task = %+v", assigned)
	}
}

func TestTasksCanMoveToNamedWorkflowLanes(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.AddTask("Implement handoff", "", "", "lane_todo", "usr_you")
	if err != nil {
		t.Fatal(err)
	}

	doing, changed, err := st.MoveTaskToLaneName(task.ID, "Doing")
	if err != nil {
		t.Fatal(err)
	}
	if !changed || doing.LaneID != "lane_doing" {
		t.Fatalf("doing task = %+v changed=%v", doing, changed)
	}
	review, changed, err := st.MoveTaskToLaneName(task.ID, "Review")
	if err != nil {
		t.Fatal(err)
	}
	if !changed || review.LaneID != "lane_review" {
		t.Fatalf("review task = %+v changed=%v", review, changed)
	}
	again, changed, err := st.MoveTaskToLaneName(task.ID, "Review")
	if err != nil {
		t.Fatal(err)
	}
	if changed || again.LaneID != "lane_review" {
		t.Fatalf("repeat move should be a no-op: %+v changed=%v", again, changed)
	}
}

func TestTaskLanesCanBeReordered(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	lane, err := st.MoveTaskLane("lane_done", 1)
	if err != nil {
		t.Fatal(err)
	}
	if lane.ID != "lane_done" {
		t.Fatalf("moved lane = %s, want lane_done", lane.ID)
	}
	lanes := st.Snapshot().TaskLanes
	if len(lanes) < 2 || lanes[1].ID != "lane_done" {
		t.Fatalf("lane order = %+v, want lane_done at index 1", lanes)
	}
	for i, lane := range lanes {
		if lane.Position != i {
			t.Fatalf("lane %s position = %d, want %d", lane.ID, lane.Position, i)
		}
	}
}

func TestDeleteTaskLaneMovesTasksToFallback(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	lane, err := st.AddTaskLane("Blocked")
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.AddTask("Wait for design", "", "", lane.ID, "usr_you")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteTaskLane(lane.ID); err != nil {
		t.Fatal(err)
	}

	snapshot := st.Snapshot()
	for _, got := range snapshot.Tasks {
		if got.ID == task.ID {
			if got.LaneID == lane.ID {
				t.Fatalf("task remained in deleted lane %s", lane.ID)
			}
			if got.LaneID == "" {
				t.Fatal("task did not move to a fallback lane")
			}
			return
		}
	}
	t.Fatal("task disappeared after deleting its lane")
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
