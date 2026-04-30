package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/xargin/open-agent-room/internal/protocol"
)

const maxAgentSkillContentBytes = 64 * 1024

type Store struct {
	mu    sync.RWMutex
	path  string
	state protocol.State
}

func New(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
		s.state = DefaultState()
		return s.saveLocked()
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &s.state); err != nil {
		return err
	}
	if s.state.Meta.ServerID == "" {
		s.state.Meta.ServerID = "srv_local"
	}
	s.ensureUserDefaultsLocked()
	s.ensureAgentRuntimeDefaultsLocked()
	s.ensureSkillLibraryLocked()
	s.ensureChannelDefaultsLocked()
	s.ensureTaskDefaultsLocked()
	return nil
}

func (s *Store) Snapshot() protocol.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := cloneState(s.state)
	hydrateAgentSkills(&snapshot)
	return snapshot
}

func (s *Store) ServerID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Meta.ServerID
}

func (s *Store) ResetRuntimePresence() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	now := protocol.Now()
	for i := range s.state.Daemons {
		if s.state.Daemons[i].Status != "offline" {
			s.state.Daemons[i].Status = "offline"
			s.state.Daemons[i].LastSeen = now
			changed = true
		}
	}
	for i := range s.state.Agents {
		if s.state.Agents[i].DaemonID != "" || s.state.Agents[i].Status == "thinking" || s.state.Agents[i].Status == "idle" || s.state.Agents[i].Status == "starting" {
			s.state.Agents[i].DaemonID = ""
			s.state.Agents[i].Status = "waiting"
			changed = true
		}
	}
	if !changed {
		return nil
	}
	s.touchLocked()
	return s.saveLocked()
}

func (s *Store) AddEnvelope(env protocol.Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Events = protocol.TrimEvents(append(s.state.Events, env), 120)
	s.touchLocked()
	return s.saveLocked()
}

func (s *Store) AddMessage(msg protocol.Message, env protocol.Envelope) (protocol.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if msg.ID == "" {
		msg.ID = protocol.NewID("msg")
	}
	if msg.Timestamp == "" {
		msg.Timestamp = protocol.Now()
	}
	if msg.Kind == "" {
		msg.Kind = "message"
	}
	msg.ProtocolID = env.ID
	s.state.Messages = protocol.TrimMessages(append(s.state.Messages, msg), 500)
	s.state.Events = protocol.TrimEvents(append(s.state.Events, env), 120)
	s.touchLocked()
	return msg, s.saveLocked()
}

func (s *Store) AddChannel(name, topic string) (protocol.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(strings.TrimPrefix(name, "#"))
	if name == "" {
		return protocol.Channel{}, errors.New("channel name is required")
	}
	var memberIDs []string
	for _, user := range s.state.Users {
		memberIDs = appendUnique(memberIDs, user.ID)
	}
	for _, agent := range s.state.Agents {
		memberIDs = appendUnique(memberIDs, agent.ID)
	}
	defaultAgentID := ""
	if len(s.state.Agents) > 0 {
		defaultAgentID = s.state.Agents[0].ID
	}
	ch := protocol.Channel{
		ID:             protocol.NewID("chan"),
		Name:           name,
		Topic:          strings.TrimSpace(topic),
		MemberIDs:      memberIDs,
		DefaultAgentID: defaultAgentID,
	}
	s.state.Channels = append(s.state.Channels, ch)
	s.touchLocked()
	return ch, s.saveLocked()
}

func (s *Store) AddUser(name string) (protocol.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return protocol.User{}, errors.New("human name is required")
	}
	user := protocol.User{
		ID:    "usr_" + slugWithFallback(name, "usr"),
		Name:  name,
		Color: userColor(len(s.state.Users)),
	}
	for _, existing := range s.state.Users {
		if existing.ID == user.ID {
			user.ID = protocol.NewID("usr")
			break
		}
	}
	s.state.Users = append(s.state.Users, user)
	for i := range s.state.Channels {
		s.state.Channels[i].MemberIDs = appendUnique(s.state.Channels[i].MemberIDs, user.ID)
	}
	s.touchLocked()
	return user, s.saveLocked()
}

func (s *Store) DeleteUser(id string) (protocol.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(id), "@"))
	if id == "" {
		return protocol.User{}, errors.New("human id is required")
	}
	for i, user := range s.state.Users {
		if !userMatches(user, id) {
			continue
		}
		if user.ID == s.state.CurrentUserID || user.ID == "usr_you" {
			return protocol.User{}, errors.New("cannot delete the current human")
		}
		s.state.Users = append(s.state.Users[:i], s.state.Users[i+1:]...)
		for j := range s.state.Channels {
			s.state.Channels[j].MemberIDs = removeValue(s.state.Channels[j].MemberIDs, user.ID)
		}
		for j := range s.state.Tasks {
			if (s.state.Tasks[j].AssigneeKind == "human" || s.state.Tasks[j].AssigneeKind == "user") && s.state.Tasks[j].AssigneeID == user.ID {
				s.state.Tasks[j].AssigneeKind = ""
				s.state.Tasks[j].AssigneeID = ""
				s.state.Tasks[j].UpdatedAt = protocol.Now()
			}
		}
		s.touchLocked()
		return user, s.saveLocked()
	}
	return protocol.User{}, errors.New("human not found")
}

func (s *Store) DeleteChannel(id string) (protocol.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(strings.TrimPrefix(id, "#"))
	if len(s.state.Channels) <= 1 {
		return protocol.Channel{}, errors.New("cannot delete the last channel")
	}
	for i, ch := range s.state.Channels {
		if ch.ID != id && ch.Name != id {
			continue
		}
		s.state.Channels = append(s.state.Channels[:i], s.state.Channels[i+1:]...)
		messages := s.state.Messages[:0]
		for _, msg := range s.state.Messages {
			if msg.ChannelID != ch.ID {
				messages = append(messages, msg)
			}
		}
		s.state.Messages = messages
		s.touchLocked()
		return ch, s.saveLocked()
	}
	return protocol.Channel{}, errors.New("channel not found")
}

func (s *Store) AddTaskLane(name string) (protocol.TaskLane, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return protocol.TaskLane{}, errors.New("lane name is required")
	}
	lane := protocol.TaskLane{
		ID:       "lane_" + slugWithFallback(name, "lane"),
		Name:     name,
		Position: len(s.state.TaskLanes),
	}
	for _, existing := range s.state.TaskLanes {
		if existing.ID == lane.ID {
			lane.ID = protocol.NewID("lane")
			break
		}
	}
	s.state.TaskLanes = append(s.state.TaskLanes, lane)
	s.normalizeTaskLanePositionsLocked()
	s.touchLocked()
	return lane, s.saveLocked()
}

func (s *Store) MoveTaskLane(id string, position int) (protocol.TaskLane, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.TaskLanes) == 0 {
		return protocol.TaskLane{}, errors.New("lane not found")
	}
	index := s.findTaskLaneIndexLocked(id)
	if index == -1 {
		return protocol.TaskLane{}, errors.New("lane not found")
	}
	if position < 0 {
		position = 0
	}
	if position >= len(s.state.TaskLanes) {
		position = len(s.state.TaskLanes) - 1
	}
	lane := s.state.TaskLanes[index]
	if index == position {
		return lane, nil
	}
	s.state.TaskLanes = append(s.state.TaskLanes[:index], s.state.TaskLanes[index+1:]...)
	if position >= len(s.state.TaskLanes) {
		s.state.TaskLanes = append(s.state.TaskLanes, lane)
	} else {
		s.state.TaskLanes = append(s.state.TaskLanes[:position], append([]protocol.TaskLane{lane}, s.state.TaskLanes[position:]...)...)
	}
	s.normalizeTaskLanePositionsLocked()
	s.touchLocked()
	if movedIndex := s.findTaskLaneIndexLocked(lane.ID); movedIndex != -1 {
		lane = s.state.TaskLanes[movedIndex]
	}
	return lane, s.saveLocked()
}

func (s *Store) DeleteTaskLane(id string) (protocol.TaskLane, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.ToLower(strings.TrimSpace(id))
	if len(s.state.TaskLanes) <= 1 {
		return protocol.TaskLane{}, errors.New("cannot delete the last lane")
	}
	index := s.findTaskLaneIndexLocked(id)
	if index == -1 {
		return protocol.TaskLane{}, errors.New("lane not found")
	}
	deleted := s.state.TaskLanes[index]
	s.state.TaskLanes = append(s.state.TaskLanes[:index], s.state.TaskLanes[index+1:]...)
	fallback := s.firstTaskLaneIDLocked()
	if fallback == "" {
		return protocol.TaskLane{}, errors.New("no fallback lane available")
	}
	for i := range s.state.Tasks {
		if s.state.Tasks[i].LaneID == deleted.ID {
			s.state.Tasks[i].LaneID = fallback
			s.state.Tasks[i].UpdatedAt = protocol.Now()
		}
	}
	s.normalizeTaskLanePositionsLocked()
	s.touchLocked()
	return deleted, s.saveLocked()
}

func (s *Store) AddTask(title, description, workdir, laneID, createdBy string) (protocol.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title = strings.TrimSpace(title)
	if title == "" {
		return protocol.Task{}, errors.New("task title is required")
	}
	laneID = strings.TrimSpace(laneID)
	if laneID == "" {
		laneID = s.firstTaskLaneIDLocked()
	}
	if !s.taskLaneExistsLocked(laneID) {
		return protocol.Task{}, errors.New("lane not found")
	}
	now := protocol.Now()
	task := protocol.Task{
		ID:          protocol.NewID("task"),
		Title:       title,
		Description: strings.TrimSpace(description),
		Workdir:     strings.TrimSpace(workdir),
		LaneID:      laneID,
		CreatedBy:   strings.TrimSpace(createdBy),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.state.Tasks = append(s.state.Tasks, task)
	s.touchLocked()
	return task, s.saveLocked()
}

func (s *Store) UpdateTask(id string, title, description, workdir, laneID, assigneeKind, assigneeID *string) (protocol.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	for i := range s.state.Tasks {
		if s.state.Tasks[i].ID != id {
			continue
		}
		if title != nil {
			next := strings.TrimSpace(*title)
			if next == "" {
				return protocol.Task{}, errors.New("task title is required")
			}
			s.state.Tasks[i].Title = next
		}
		if description != nil {
			s.state.Tasks[i].Description = strings.TrimSpace(*description)
		}
		if workdir != nil {
			s.state.Tasks[i].Workdir = strings.TrimSpace(*workdir)
		}
		if laneID != nil {
			next := strings.TrimSpace(*laneID)
			if next == "" || !s.taskLaneExistsLocked(next) {
				return protocol.Task{}, errors.New("lane not found")
			}
			s.state.Tasks[i].LaneID = next
		}
		if assigneeKind != nil || assigneeID != nil {
			nextKind := s.state.Tasks[i].AssigneeKind
			nextID := s.state.Tasks[i].AssigneeID
			if assigneeKind != nil {
				nextKind = *assigneeKind
			}
			if assigneeID != nil {
				nextID = *assigneeID
			}
			normalizedKind, normalizedID, err := s.normalizeTaskAssigneeLocked(nextKind, nextID)
			if err != nil {
				return protocol.Task{}, err
			}
			s.state.Tasks[i].AssigneeKind = normalizedKind
			s.state.Tasks[i].AssigneeID = normalizedID
		}
		s.state.Tasks[i].UpdatedAt = protocol.Now()
		s.touchLocked()
		return s.state.Tasks[i], s.saveLocked()
	}
	return protocol.Task{}, errors.New("task not found")
}

func (s *Store) MoveTaskToLaneName(id, laneName string) (protocol.Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	laneIndex := s.findTaskLaneIndexLocked(laneName)
	if laneIndex == -1 {
		return protocol.Task{}, false, errors.New("lane not found")
	}
	targetID := s.state.TaskLanes[laneIndex].ID
	for i := range s.state.Tasks {
		if s.state.Tasks[i].ID != strings.TrimSpace(id) {
			continue
		}
		if s.state.Tasks[i].LaneID == targetID {
			return s.state.Tasks[i], false, nil
		}
		s.state.Tasks[i].LaneID = targetID
		s.state.Tasks[i].UpdatedAt = protocol.Now()
		s.touchLocked()
		return s.state.Tasks[i], true, s.saveLocked()
	}
	return protocol.Task{}, false, errors.New("task not found")
}

func (s *Store) AssignTaskByChannel(channelID, kind, assigneeID string) (protocol.Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channelID = strings.TrimSpace(channelID)
	normalizedKind, normalizedID, err := s.normalizeTaskAssigneeLocked(kind, assigneeID)
	if err != nil {
		return protocol.Task{}, false, err
	}
	for i := range s.state.Tasks {
		if s.state.Tasks[i].ChannelID != channelID {
			continue
		}
		if s.state.Tasks[i].AssigneeKind == normalizedKind && s.state.Tasks[i].AssigneeID == normalizedID {
			return s.state.Tasks[i], false, nil
		}
		s.state.Tasks[i].AssigneeKind = normalizedKind
		s.state.Tasks[i].AssigneeID = normalizedID
		s.state.Tasks[i].UpdatedAt = protocol.Now()
		s.touchLocked()
		return s.state.Tasks[i], true, s.saveLocked()
	}
	return protocol.Task{}, false, nil
}

func (s *Store) TaskForChannel(channelID string) (protocol.Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	channelID = strings.TrimSpace(channelID)
	for _, task := range s.state.Tasks {
		if task.ChannelID == channelID {
			return task, true
		}
	}
	return protocol.Task{}, false
}

func (s *Store) DeleteTask(id string) (protocol.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	for i, task := range s.state.Tasks {
		if task.ID != id {
			continue
		}
		s.state.Tasks = append(s.state.Tasks[:i], s.state.Tasks[i+1:]...)
		s.touchLocked()
		return task, s.saveLocked()
	}
	return protocol.Task{}, errors.New("task not found")
}

func (s *Store) CreateTaskChannel(taskID string) (protocol.Task, protocol.Channel, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	taskID = strings.TrimSpace(taskID)
	for i := range s.state.Tasks {
		if s.state.Tasks[i].ID != taskID {
			continue
		}
		if s.state.Tasks[i].ChannelID != "" {
			if ch, ok := s.findChannelLocked(s.state.Tasks[i].ChannelID); ok {
				return s.state.Tasks[i], ch, false, nil
			}
		}
		name := s.uniqueChannelNameLocked("task-" + slugWithFallback(s.state.Tasks[i].Title, "task"))
		ch := protocol.Channel{
			ID:             protocol.NewID("chan"),
			Name:           name,
			Topic:          "Task discussion: " + s.state.Tasks[i].Title,
			MemberIDs:      s.allParticipantIDsLocked(),
			DefaultAgentID: s.firstAgentIDLocked(),
		}
		s.state.Channels = append(s.state.Channels, ch)
		s.state.Tasks[i].ChannelID = ch.ID
		s.state.Tasks[i].UpdatedAt = protocol.Now()
		s.touchLocked()
		return s.state.Tasks[i], ch, true, s.saveLocked()
	}
	return protocol.Task{}, protocol.Channel{}, false, errors.New("task not found")
}

func (s *Store) AddAgent(name, persona, systemPrompt, runtimeName, model string, skills []protocol.AgentSkill, existingSkillIDs []string) (protocol.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return protocol.Agent{}, errors.New("agent name is required")
	}
	skillIDs, err := addSkillsLocked(&s.state, skills)
	if err != nil {
		return protocol.Agent{}, err
	}
	for _, skillID := range existingSkillIDs {
		skill, ok := findSkill(s.state.Skills, skillID)
		if !ok {
			return protocol.Agent{}, errors.New("skill not found")
		}
		skillIDs = appendUnique(skillIDs, skill.ID)
	}
	agent := protocol.Agent{
		ID:           "agent_" + slug(name),
		Name:         name,
		Persona:      strings.TrimSpace(persona),
		SystemPrompt: strings.TrimSpace(systemPrompt),
		Runtime:      normalizeRuntime(runtimeName),
		Model:        strings.TrimSpace(model),
		Status:       "waiting",
		Capabilities: []string{"reply", "remember", "tasks"},
		SkillIDs:     skillIDs,
		Color:        agentColor(len(s.state.Agents)),
		LastSeen:     protocol.Now(),
	}
	if agent.Persona == "" {
		agent.Persona = "General collaboration agent"
	}
	for i := range s.state.Agents {
		if s.state.Agents[i].ID == agent.ID {
			agent.ID = protocol.NewID("agent")
			break
		}
	}
	s.state.Agents = append(s.state.Agents, agent)
	for i := range s.state.Channels {
		s.state.Channels[i].MemberIDs = appendUnique(s.state.Channels[i].MemberIDs, agent.ID)
		if s.state.Channels[i].DefaultAgentID == "" {
			s.state.Channels[i].DefaultAgentID = agent.ID
		}
	}
	s.touchLocked()
	return hydrateAgentSkill(agent, s.state.Skills), s.saveLocked()
}

func (s *Store) DeleteAgent(id string) (protocol.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(id), "@"))
	for i, agent := range s.state.Agents {
		if !agentMatches(agent, id) {
			continue
		}
		s.state.Agents = append(s.state.Agents[:i], s.state.Agents[i+1:]...)
		for j := range s.state.Channels {
			s.state.Channels[j].MemberIDs = removeValue(s.state.Channels[j].MemberIDs, agent.ID)
			if s.state.Channels[j].DefaultAgentID == agent.ID {
				s.state.Channels[j].DefaultAgentID = firstChannelAgentID(s.state.Channels[j], s.state.Agents)
				if s.state.Channels[j].DefaultAgentID != "" {
					s.state.Channels[j].MemberIDs = appendUnique(s.state.Channels[j].MemberIDs, s.state.Channels[j].DefaultAgentID)
				}
			}
		}
		for j := range s.state.Tasks {
			if s.state.Tasks[j].AssigneeKind == "agent" && s.state.Tasks[j].AssigneeID == agent.ID {
				s.state.Tasks[j].AssigneeKind = ""
				s.state.Tasks[j].AssigneeID = ""
				s.state.Tasks[j].UpdatedAt = protocol.Now()
			}
		}
		s.touchLocked()
		return agent, s.saveLocked()
	}
	return protocol.Agent{}, errors.New("agent not found")
}

func (s *Store) UpdateChannelDefaultAgent(channelID, agentID string) (protocol.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var agent protocol.Agent
	agentFound := false
	for _, existing := range s.state.Agents {
		if existing.ID == agentID {
			agent = existing
			agentFound = true
			break
		}
	}
	if !agentFound {
		return protocol.Channel{}, errors.New("agent not found")
	}

	for i := range s.state.Channels {
		if s.state.Channels[i].ID == channelID {
			s.state.Channels[i].DefaultAgentID = agent.ID
			s.state.Channels[i].MemberIDs = appendUnique(s.state.Channels[i].MemberIDs, agent.ID)
			s.touchLocked()
			return s.state.Channels[i], s.saveLocked()
		}
	}
	return protocol.Channel{}, errors.New("channel not found")
}

func (s *Store) AddSkill(name, source, content string, tags []string) (protocol.AgentSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	skill, err := addSkillLocked(&s.state, name, source, content, tags)
	if err != nil {
		return protocol.AgentSkill{}, err
	}
	s.touchLocked()
	return skill, s.saveLocked()
}

func (s *Store) DeleteSkill(skillID string) (protocol.AgentSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if skillID == "" {
		return protocol.AgentSkill{}, errors.New("skill id is required")
	}
	for i, skill := range s.state.Skills {
		if !skillMatches(skill, skillID) {
			continue
		}
		s.state.Skills = append(s.state.Skills[:i], s.state.Skills[i+1:]...)
		for j := range s.state.Agents {
			s.state.Agents[j].SkillIDs = removeValue(s.state.Agents[j].SkillIDs, skill.ID)
			s.state.Agents[j].Skills = removeSkillValue(s.state.Agents[j].Skills, skill.ID)
		}
		s.touchLocked()
		return skill, s.saveLocked()
	}
	return protocol.AgentSkill{}, errors.New("skill not found")
}

func (s *Store) AddAgentSkill(agentID, name, source, content string, tags []string) (protocol.AgentSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agentID = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(agentID), "@"))
	for i := range s.state.Agents {
		if !agentMatches(s.state.Agents[i], agentID) {
			continue
		}
		skill, err := addSkillLocked(&s.state, name, source, content, tags)
		if err != nil {
			return protocol.AgentSkill{}, err
		}
		s.state.Agents[i].SkillIDs = appendUnique(s.state.Agents[i].SkillIDs, skill.ID)
		s.state.Agents[i].Skills = nil
		s.touchLocked()
		return skill, s.saveLocked()
	}
	return protocol.AgentSkill{}, errors.New("agent not found")
}

func (s *Store) AttachAgentSkill(agentID, skillID string) (protocol.AgentSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agentID = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(agentID), "@"))
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if agentID == "" || skillID == "" {
		return protocol.AgentSkill{}, errors.New("agent id and skill id are required")
	}
	skill, ok := findSkill(s.state.Skills, skillID)
	if !ok {
		return protocol.AgentSkill{}, errors.New("skill not found")
	}
	for i := range s.state.Agents {
		if !agentMatches(s.state.Agents[i], agentID) {
			continue
		}
		s.state.Agents[i].SkillIDs = appendUnique(s.state.Agents[i].SkillIDs, skill.ID)
		s.state.Agents[i].Skills = nil
		s.touchLocked()
		return skill, s.saveLocked()
	}
	return protocol.AgentSkill{}, errors.New("agent not found")
}

func (s *Store) DeleteAgentSkill(agentID, skillID string) (protocol.AgentSkill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agentID = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(agentID), "@"))
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if agentID == "" || skillID == "" {
		return protocol.AgentSkill{}, errors.New("agent id and skill id are required")
	}
	for i := range s.state.Agents {
		if !agentMatches(s.state.Agents[i], agentID) {
			continue
		}
		if skill, ok := findSkill(s.state.Skills, skillID); ok {
			if !containsValue(s.state.Agents[i].SkillIDs, skill.ID) && !agentHasLegacySkill(s.state.Agents[i], skill.ID) {
				return protocol.AgentSkill{}, errors.New("skill not attached to agent")
			}
			s.state.Agents[i].SkillIDs = removeValue(s.state.Agents[i].SkillIDs, skill.ID)
			s.state.Agents[i].Skills = removeSkillValue(s.state.Agents[i].Skills, skill.ID)
			s.touchLocked()
			return skill, s.saveLocked()
		}
		return protocol.AgentSkill{}, errors.New("skill not found")
	}
	return protocol.AgentSkill{}, errors.New("agent not found")
}

func (s *Store) UpsertDaemon(d protocol.Daemon) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Daemons {
		if s.state.Daemons[i].ID == d.ID {
			s.state.Daemons[i] = d
			s.touchLocked()
			return s.saveLocked()
		}
	}
	s.state.Daemons = append(s.state.Daemons, d)
	s.touchLocked()
	return s.saveLocked()
}

func (s *Store) DisconnectDaemon(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Daemons {
		if s.state.Daemons[i].ID == id {
			s.state.Daemons[i].Status = "offline"
			s.state.Daemons[i].LastSeen = protocol.Now()
		}
	}
	for i := range s.state.Agents {
		if s.state.Agents[i].DaemonID == id {
			s.state.Agents[i].Status = "waiting"
		}
	}
	s.touchLocked()
	return s.saveLocked()
}

func (s *Store) UpdateAgentStatus(agentID, status, daemonID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Agents {
		if s.state.Agents[i].ID == agentID {
			s.state.Agents[i].Status = status
			if daemonID != "" {
				s.state.Agents[i].DaemonID = daemonID
			}
			s.state.Agents[i].LastSeen = protocol.Now()
			s.touchLocked()
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Store) AppendMemory(agentID, text, source string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Agents {
		if s.state.Agents[i].ID == agentID {
			item := protocol.MemoryItem{
				ID:        protocol.NewID("mem"),
				Text:      text,
				Source:    source,
				CreatedAt: protocol.Now(),
			}
			s.state.Agents[i].Memory = append(s.state.Agents[i].Memory, item)
			if len(s.state.Agents[i].Memory) > 8 {
				s.state.Agents[i].Memory = s.state.Agents[i].Memory[len(s.state.Agents[i].Memory)-8:]
			}
			s.touchLocked()
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Store) FindAgent(id string) (protocol.Agent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id = strings.ToLower(strings.TrimPrefix(id, "@"))
	for _, agent := range s.state.Agents {
		if agentMatches(agent, id) {
			return hydrateAgentSkill(agent, s.state.Skills), true
		}
	}
	return protocol.Agent{}, false
}

func (s *Store) FindChannel(id string) (protocol.Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.state.Channels {
		if ch.ID == id || ch.Name == strings.TrimPrefix(id, "#") {
			return ch, true
		}
	}
	return protocol.Channel{}, false
}

func (s *Store) RecentMessages(channelID string, limit int) []protocol.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []protocol.Message
	for i := len(s.state.Messages) - 1; i >= 0 && len(out) < limit; i-- {
		if s.state.Messages[i].ChannelID == channelID {
			out = append(out, s.state.Messages[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (s *Store) touchLocked() {
	s.state.Meta.UpdatedAt = protocol.Now()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

func DefaultState() protocol.State {
	now := protocol.Now()
	return protocol.State{
		Meta: protocol.Meta{
			Version:   1,
			ServerID:  "srv_local",
			CreatedAt: now,
			UpdatedAt: now,
		},
		CurrentUserID: "usr_you",
		Users: []protocol.User{
			{ID: "usr_you", Name: "You", Color: "#2563eb"},
		},
		Channels: []protocol.Channel{
			{ID: "chan_general", Name: "general", Topic: "Daily human-agent collaboration", MemberIDs: []string{"usr_you", "agent_ada", "agent_lin"}, DefaultAgentID: "agent_ada"},
			{ID: "chan_build-room", Name: "build-room", Topic: "Implementation tasks, reviews, and handoffs", MemberIDs: []string{"usr_you", "agent_lin", "agent_ada"}, DefaultAgentID: "agent_lin"},
		},
		TaskLanes: defaultTaskLanes(),
		Agents: []protocol.Agent{
			{ID: "agent_ada", Name: "Ada", Persona: "Systems designer who turns rough requests into concrete plans.", Runtime: "codex", Status: "waiting", Capabilities: []string{"plan", "review", "remember"}, Color: "#0f766e"},
			{ID: "agent_lin", Name: "Lin", Persona: "Implementation agent focused on small verified changes.", Runtime: "codex", Status: "waiting", Capabilities: []string{"implement", "test", "summarize"}, Color: "#b45309"},
		},
		Messages: []protocol.Message{
			{ID: protocol.NewID("msg"), ChannelID: "chan_general", AuthorKind: "system", AuthorID: "system", AuthorName: "System", Text: "Workspace created. Connect the local daemon, then mention @Ada or @Lin.", Kind: "system", Timestamp: now},
		},
	}
}

func defaultTaskLanes() []protocol.TaskLane {
	return []protocol.TaskLane{
		{ID: "lane_backlog", Name: "Backlog", Position: 0},
		{ID: "lane_todo", Name: "Todo", Position: 1},
		{ID: "lane_doing", Name: "Doing", Position: 2},
		{ID: "lane_review", Name: "Review", Position: 3},
		{ID: "lane_done", Name: "Done", Position: 4},
		{ID: "lane_unplanned", Name: "Unplanned", Position: 5},
	}
}

func cloneState(in protocol.State) protocol.State {
	b, _ := json.Marshal(in)
	var out protocol.State
	_ = json.Unmarshal(b, &out)
	return out
}

func hydrateAgentSkills(state *protocol.State) {
	for i := range state.Agents {
		state.Agents[i] = hydrateAgentSkill(state.Agents[i], state.Skills)
	}
}

func hydrateAgentSkill(agent protocol.Agent, skills []protocol.AgentSkill) protocol.Agent {
	var out []protocol.AgentSkill
	for _, skillID := range agent.SkillIDs {
		if skill, ok := findSkill(skills, skillID); ok {
			out = append(out, skill)
		}
	}
	if len(out) == 0 && len(agent.SkillIDs) == 0 && len(agent.Skills) > 0 {
		return agent
	}
	agent.Skills = out
	return agent
}

func appendUnique(values []string, next string) []string {
	for _, v := range values {
		if v == next {
			return values
		}
	}
	return append(values, next)
}

func containsValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func removeValue(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func removeSkillValue(values []protocol.AgentSkill, target string) []protocol.AgentSkill {
	out := values[:0]
	for _, value := range values {
		if !skillMatches(value, target) {
			out = append(out, value)
		}
	}
	return out
}

func agentMatches(agent protocol.Agent, id string) bool {
	return strings.ToLower(agent.ID) == id || strings.ToLower(agent.Name) == id || strings.TrimPrefix(strings.ToLower(agent.ID), "agent_") == id
}

func userMatches(user protocol.User, id string) bool {
	return strings.ToLower(user.ID) == id || strings.ToLower(user.Name) == id || strings.TrimPrefix(strings.ToLower(user.ID), "usr_") == id
}

func (s *Store) normalizeTaskAssigneeLocked(kind, id string) (string, string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	id = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(id), "@"))
	if id == "" {
		return "", "", nil
	}
	if kind == "user" {
		kind = "human"
	}
	if kind == "" {
		if _, ok := s.findAgentLocked(id); ok {
			return "agent", id, nil
		}
		if _, ok := s.findUserLocked(id); ok {
			return "human", id, nil
		}
		return "", "", errors.New("assignee not found")
	}
	switch kind {
	case "agent":
		agent, ok := s.findAgentLocked(id)
		if !ok {
			return "", "", errors.New("agent not found")
		}
		return "agent", agent.ID, nil
	case "human":
		user, ok := s.findUserLocked(id)
		if !ok {
			return "", "", errors.New("human not found")
		}
		return "human", user.ID, nil
	default:
		return "", "", errors.New("assignee kind must be agent or human")
	}
}

func (s *Store) findAgentLocked(id string) (protocol.Agent, bool) {
	id = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(id), "@"))
	for _, agent := range s.state.Agents {
		if agentMatches(agent, id) {
			return agent, true
		}
	}
	return protocol.Agent{}, false
}

func (s *Store) findUserLocked(id string) (protocol.User, bool) {
	id = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(id), "@"))
	for _, user := range s.state.Users {
		if userMatches(user, id) {
			return user, true
		}
	}
	return protocol.User{}, false
}

func skillMatches(skill protocol.AgentSkill, id string) bool {
	return strings.ToLower(skill.ID) == id || strings.ToLower(skill.Name) == id || strings.TrimPrefix(strings.ToLower(skill.ID), "skill_") == id
}

func findSkill(skills []protocol.AgentSkill, id string) (protocol.AgentSkill, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, skill := range skills {
		if skillMatches(skill, id) {
			return skill, true
		}
	}
	return protocol.AgentSkill{}, false
}

func (s *Store) findTaskLaneIndexLocked(id string) int {
	id = strings.ToLower(strings.TrimSpace(id))
	for i, lane := range s.state.TaskLanes {
		if strings.ToLower(lane.ID) == id || strings.ToLower(lane.Name) == id || strings.TrimPrefix(strings.ToLower(lane.ID), "lane_") == id {
			return i
		}
	}
	return -1
}

func (s *Store) taskLaneExistsLocked(id string) bool {
	return s.findTaskLaneIndexLocked(id) != -1
}

func (s *Store) firstTaskLaneIDLocked() string {
	if len(s.state.TaskLanes) == 0 {
		return ""
	}
	for _, lane := range s.state.TaskLanes {
		if lane.ID == "lane_unplanned" {
			return lane.ID
		}
	}
	return s.state.TaskLanes[0].ID
}

func (s *Store) normalizeTaskLanePositionsLocked() {
	for i := range s.state.TaskLanes {
		s.state.TaskLanes[i].Position = i
	}
}

func (s *Store) ensureTaskLaneLocked(lane protocol.TaskLane, afterID, beforeID string) {
	if s.findTaskLaneIndexLocked(lane.ID) != -1 || s.findTaskLaneIndexLocked(lane.Name) != -1 {
		return
	}
	insertAt := len(s.state.TaskLanes)
	if afterIndex := s.findTaskLaneIndexLocked(afterID); afterIndex != -1 {
		insertAt = afterIndex + 1
	} else if beforeIndex := s.findTaskLaneIndexLocked(beforeID); beforeIndex != -1 {
		insertAt = beforeIndex
	}
	if insertAt >= len(s.state.TaskLanes) {
		s.state.TaskLanes = append(s.state.TaskLanes, lane)
		return
	}
	s.state.TaskLanes = append(s.state.TaskLanes[:insertAt], append([]protocol.TaskLane{lane}, s.state.TaskLanes[insertAt:]...)...)
}

func (s *Store) findChannelLocked(id string) (protocol.Channel, bool) {
	id = strings.TrimSpace(strings.TrimPrefix(id, "#"))
	for _, ch := range s.state.Channels {
		if ch.ID == id || ch.Name == id {
			return ch, true
		}
	}
	return protocol.Channel{}, false
}

func (s *Store) allParticipantIDsLocked() []string {
	var memberIDs []string
	for _, user := range s.state.Users {
		memberIDs = appendUnique(memberIDs, user.ID)
	}
	for _, agent := range s.state.Agents {
		memberIDs = appendUnique(memberIDs, agent.ID)
	}
	return memberIDs
}

func (s *Store) firstAgentIDLocked() string {
	if len(s.state.Agents) == 0 {
		return ""
	}
	return s.state.Agents[0].ID
}

func (s *Store) uniqueChannelNameLocked(base string) string {
	base = strings.Trim(slugWithFallback(base, "task"), "-")
	if base == "" {
		base = strings.TrimPrefix(protocol.NewID("task"), "task_")
	}
	candidate := base
	for suffix := 2; ; suffix++ {
		if _, ok := s.findChannelLocked(candidate); !ok {
			return candidate
		}
		candidate = base + "-" + strconv.Itoa(suffix)
	}
}

func agentHasLegacySkill(agent protocol.Agent, skillID string) bool {
	for _, skill := range agent.Skills {
		if skillMatches(skill, skillID) {
			return true
		}
	}
	return false
}

func addSkillsLocked(state *protocol.State, skills []protocol.AgentSkill) ([]string, error) {
	var ids []string
	for _, skill := range skills {
		normalized, err := addSkillLocked(state, skill.Name, skill.Source, skill.Content, skill.Tags)
		if err != nil {
			return nil, err
		}
		ids = appendUnique(ids, normalized.ID)
	}
	return ids, nil
}

func addSkillLocked(state *protocol.State, name, source, content string, tags []string) (protocol.AgentSkill, error) {
	skill, err := newAgentSkill(name, source, content, tags, state.Skills)
	if err != nil {
		return protocol.AgentSkill{}, err
	}
	state.Skills = append(state.Skills, skill)
	return skill, nil
}

func newAgentSkill(name, source, content string, tags []string, existing []protocol.AgentSkill) (protocol.AgentSkill, error) {
	name = strings.TrimSpace(name)
	source = strings.TrimSpace(source)
	content = strings.TrimSpace(content)
	if name == "" {
		return protocol.AgentSkill{}, errors.New("skill name is required")
	}
	if content == "" {
		return protocol.AgentSkill{}, errors.New("skill content is required")
	}
	if len([]byte(content)) > maxAgentSkillContentBytes {
		return protocol.AgentSkill{}, errors.New("skill content is too large")
	}
	skill := protocol.AgentSkill{
		ID:        "skill_" + slugWithFallback(name, "skill"),
		Name:      name,
		Source:    source,
		Content:   content,
		Tags:      normalizeSkillTags(tags),
		CreatedAt: protocol.Now(),
	}
	for _, candidate := range existing {
		if candidate.ID == skill.ID {
			skill.ID = protocol.NewID("skill")
			break
		}
	}
	return skill, nil
}

func normalizeSkillTags(tags []string) []string {
	var out []string
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(tag, "#")))
		normalized = strings.Join(strings.Fields(normalized), "-")
		normalized = strings.Trim(normalized, "-")
		if normalized == "" {
			continue
		}
		normalized = strings.Trim(truncateRunes(normalized, 32), "-")
		out = appendUnique(out, normalized)
	}
	return out
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func slug(value string) string {
	return slugWithFallback(value, "agent")
}

func slugWithFallback(value, prefix string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return strings.TrimPrefix(protocol.NewID(prefix), prefix+"_")
	}
	return out
}

func agentColor(idx int) string {
	colors := []string{"#0f766e", "#b45309", "#4f46e5", "#be123c", "#0369a1", "#7c2d12"}
	return colors[idx%len(colors)]
}

func userColor(idx int) string {
	colors := []string{"#2563eb", "#7c3aed", "#0891b2", "#9333ea", "#dc2626", "#475569"}
	return colors[idx%len(colors)]
}

func (s *Store) ensureUserDefaultsLocked() {
	if s.state.CurrentUserID == "" {
		s.state.CurrentUserID = "usr_you"
	}
	for i := range s.state.Users {
		if s.state.Users[i].Color == "" {
			s.state.Users[i].Color = userColor(i)
		}
	}
	for _, user := range s.state.Users {
		if user.ID == s.state.CurrentUserID {
			return
		}
	}
	s.state.Users = append([]protocol.User{{ID: s.state.CurrentUserID, Name: "You", Color: userColor(0)}}, s.state.Users...)
}

func (s *Store) ensureAgentRuntimeDefaultsLocked() {
	for i := range s.state.Agents {
		s.state.Agents[i].Runtime = normalizeRuntime(s.state.Agents[i].Runtime)
		s.state.Agents[i].Model = strings.TrimSpace(s.state.Agents[i].Model)
	}
}

func (s *Store) ensureSkillLibraryLocked() {
	for i := range s.state.Skills {
		if strings.TrimSpace(s.state.Skills[i].ID) == "" {
			s.state.Skills[i].ID = protocol.NewID("skill")
		}
		s.state.Skills[i].Tags = normalizeSkillTags(s.state.Skills[i].Tags)
		if strings.TrimSpace(s.state.Skills[i].CreatedAt) == "" {
			s.state.Skills[i].CreatedAt = protocol.Now()
		}
	}
	for i := range s.state.Agents {
		var skillIDs []string
		for _, skillID := range s.state.Agents[i].SkillIDs {
			if skill, ok := findSkill(s.state.Skills, skillID); ok {
				skillIDs = appendUnique(skillIDs, skill.ID)
			}
		}
		for _, skill := range s.state.Agents[i].Skills {
			if strings.TrimSpace(skill.ID) != "" {
				if existing, ok := findSkill(s.state.Skills, skill.ID); ok {
					skillIDs = appendUnique(skillIDs, existing.ID)
					continue
				}
			}
			normalized, err := newAgentSkill(skill.Name, skill.Source, skill.Content, skill.Tags, s.state.Skills)
			if err != nil {
				continue
			}
			s.state.Skills = append(s.state.Skills, normalized)
			skillIDs = appendUnique(skillIDs, normalized.ID)
		}
		s.state.Agents[i].SkillIDs = skillIDs
		s.state.Agents[i].Skills = nil
	}
}

func (s *Store) ensureTaskDefaultsLocked() {
	if len(s.state.TaskLanes) == 0 {
		s.state.TaskLanes = defaultTaskLanes()
	}
	seen := make(map[string]bool, len(s.state.TaskLanes))
	for i := range s.state.TaskLanes {
		if strings.TrimSpace(s.state.TaskLanes[i].ID) == "" {
			s.state.TaskLanes[i].ID = "lane_" + slugWithFallback(s.state.TaskLanes[i].Name, "lane")
		}
		if strings.TrimSpace(s.state.TaskLanes[i].Name) == "" {
			s.state.TaskLanes[i].Name = "Lane"
		}
		if seen[s.state.TaskLanes[i].ID] {
			s.state.TaskLanes[i].ID = protocol.NewID("lane")
		}
		seen[s.state.TaskLanes[i].ID] = true
	}
	s.ensureTaskLaneLocked(protocol.TaskLane{ID: "lane_review", Name: "Review"}, "lane_doing", "lane_done")
	s.normalizeTaskLanePositionsLocked()

	fallback := s.firstTaskLaneIDLocked()
	for i := range s.state.Tasks {
		if s.state.Tasks[i].ID == "" {
			s.state.Tasks[i].ID = protocol.NewID("task")
		}
		if strings.TrimSpace(s.state.Tasks[i].Title) == "" {
			s.state.Tasks[i].Title = "Untitled task"
		}
		if s.state.Tasks[i].LaneID == "" || !s.taskLaneExistsLocked(s.state.Tasks[i].LaneID) {
			s.state.Tasks[i].LaneID = fallback
		}
		if s.state.Tasks[i].CreatedAt == "" {
			s.state.Tasks[i].CreatedAt = protocol.Now()
		}
		if s.state.Tasks[i].UpdatedAt == "" {
			s.state.Tasks[i].UpdatedAt = s.state.Tasks[i].CreatedAt
		}
		kind, id, err := s.normalizeTaskAssigneeLocked(s.state.Tasks[i].AssigneeKind, s.state.Tasks[i].AssigneeID)
		if err != nil {
			kind, id = "", ""
		}
		s.state.Tasks[i].AssigneeKind = kind
		s.state.Tasks[i].AssigneeID = id
	}
}

func normalizeRuntime(runtimeName string) string {
	switch strings.ToLower(strings.TrimSpace(runtimeName)) {
	case "claude":
		return "claude"
	case "demo":
		return "demo"
	case "codex", "":
		return "codex"
	default:
		return "codex"
	}
}

func (s *Store) ensureChannelDefaultsLocked() {
	agentIDs := make(map[string]bool, len(s.state.Agents))
	for _, agent := range s.state.Agents {
		agentIDs[agent.ID] = true
	}
	for i := range s.state.Channels {
		for _, user := range s.state.Users {
			s.state.Channels[i].MemberIDs = appendUnique(s.state.Channels[i].MemberIDs, user.ID)
		}
		if agentIDs[s.state.Channels[i].DefaultAgentID] {
			s.state.Channels[i].MemberIDs = appendUnique(s.state.Channels[i].MemberIDs, s.state.Channels[i].DefaultAgentID)
			continue
		}
		s.state.Channels[i].DefaultAgentID = firstChannelAgentID(s.state.Channels[i], s.state.Agents)
		if s.state.Channels[i].DefaultAgentID != "" {
			s.state.Channels[i].MemberIDs = appendUnique(s.state.Channels[i].MemberIDs, s.state.Channels[i].DefaultAgentID)
		}
	}
}

func firstChannelAgentID(ch protocol.Channel, agents []protocol.Agent) string {
	agentIDs := make(map[string]bool, len(agents))
	for _, agent := range agents {
		agentIDs[agent.ID] = true
	}
	for _, memberID := range ch.MemberIDs {
		if agentIDs[memberID] {
			return memberID
		}
	}
	if len(agents) == 0 {
		return ""
	}
	return agents[0].ID
}
