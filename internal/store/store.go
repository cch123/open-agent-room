package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xargin/open-agent-room/internal/protocol"
)

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
	s.ensureChannelDefaultsLocked()
	return nil
}

func (s *Store) Snapshot() protocol.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
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

func (s *Store) AddAgent(name, persona, runtimeName, model string) (protocol.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return protocol.Agent{}, errors.New("agent name is required")
	}
	agent := protocol.Agent{
		ID:           "agent_" + slug(name),
		Name:         name,
		Persona:      strings.TrimSpace(persona),
		Runtime:      normalizeRuntime(runtimeName),
		Model:        strings.TrimSpace(model),
		Status:       "waiting",
		Capabilities: []string{"reply", "remember", "tasks"},
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
	return agent, s.saveLocked()
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
			return agent, true
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
		Agents: []protocol.Agent{
			{ID: "agent_ada", Name: "Ada", Persona: "Systems designer who turns rough requests into concrete plans.", Runtime: "codex", Status: "waiting", Capabilities: []string{"plan", "review", "remember"}, Color: "#0f766e"},
			{ID: "agent_lin", Name: "Lin", Persona: "Implementation agent focused on small verified changes.", Runtime: "codex", Status: "waiting", Capabilities: []string{"implement", "test", "summarize"}, Color: "#b45309"},
		},
		Messages: []protocol.Message{
			{ID: protocol.NewID("msg"), ChannelID: "chan_general", AuthorKind: "system", AuthorID: "system", AuthorName: "System", Text: "Workspace created. Connect the local daemon, then mention @Ada or @Lin.", Kind: "system", Timestamp: now},
		},
	}
}

func cloneState(in protocol.State) protocol.State {
	b, _ := json.Marshal(in)
	var out protocol.State
	_ = json.Unmarshal(b, &out)
	return out
}

func appendUnique(values []string, next string) []string {
	for _, v := range values {
		if v == next {
			return values
		}
	}
	return append(values, next)
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

func agentMatches(agent protocol.Agent, id string) bool {
	return strings.ToLower(agent.ID) == id || strings.ToLower(agent.Name) == id || strings.TrimPrefix(strings.ToLower(agent.ID), "agent_") == id
}

func userMatches(user protocol.User, id string) bool {
	return strings.ToLower(user.ID) == id || strings.ToLower(user.Name) == id || strings.TrimPrefix(strings.ToLower(user.ID), "usr_") == id
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
