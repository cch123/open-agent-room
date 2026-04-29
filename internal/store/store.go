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
	ch := protocol.Channel{
		ID:        protocol.NewID("chan"),
		Name:      name,
		Topic:     strings.TrimSpace(topic),
		MemberIDs: []string{"usr_you"},
	}
	s.state.Channels = append(s.state.Channels, ch)
	s.touchLocked()
	return ch, s.saveLocked()
}

func (s *Store) AddAgent(name, persona string) (protocol.Agent, error) {
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
	}
	s.touchLocked()
	return agent, s.saveLocked()
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
		if strings.ToLower(agent.ID) == id || strings.ToLower(agent.Name) == id || strings.TrimPrefix(strings.ToLower(agent.ID), "agent_") == id {
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
			{ID: "chan_general", Name: "general", Topic: "Daily human-agent collaboration", MemberIDs: []string{"usr_you", "agent_ada", "agent_lin"}},
			{ID: "chan_build-room", Name: "build-room", Topic: "Implementation tasks, reviews, and handoffs", MemberIDs: []string{"usr_you", "agent_ada", "agent_lin"}},
		},
		Agents: []protocol.Agent{
			{ID: "agent_ada", Name: "Ada", Persona: "Systems designer who turns rough requests into concrete plans.", Status: "waiting", Capabilities: []string{"plan", "review", "remember"}, Color: "#0f766e"},
			{ID: "agent_lin", Name: "Lin", Persona: "Implementation agent focused on small verified changes.", Status: "waiting", Capabilities: []string{"implement", "test", "summarize"}, Color: "#b45309"},
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

func slug(value string) string {
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
		return protocol.NewID("agent")
	}
	return out
}

func agentColor(idx int) string {
	colors := []string{"#0f766e", "#b45309", "#4f46e5", "#be123c", "#0369a1", "#7c2d12"}
	return colors[idx%len(colors)]
}
