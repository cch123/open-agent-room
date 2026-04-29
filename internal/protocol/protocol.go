package protocol

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

type Actor struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type Scope struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Trace struct {
	CorrelationID string `json:"correlationId"`
	CausationID   string `json:"causationId,omitempty"`
}

type Envelope struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	TS       string          `json:"ts"`
	ServerID string          `json:"serverId,omitempty"`
	Actor    Actor           `json:"actor"`
	Scope    Scope           `json:"scope"`
	Payload  json.RawMessage `json:"payload"`
	Trace    Trace           `json:"trace"`
}

type Meta struct {
	Version   int    `json:"version"`
	ServerID  string `json:"serverId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Channel struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Topic          string   `json:"topic"`
	MemberIDs      []string `json:"memberIds"`
	DefaultAgentID string   `json:"defaultAgentId,omitempty"`
}

type MemoryItem struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
}

type Agent struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Persona      string       `json:"persona"`
	Runtime      string       `json:"runtime"`
	Model        string       `json:"model,omitempty"`
	Status       string       `json:"status"`
	DaemonID     string       `json:"daemonId,omitempty"`
	Capabilities []string     `json:"capabilities"`
	Memory       []MemoryItem `json:"memory"`
	Color        string       `json:"color"`
	LastSeen     string       `json:"lastSeen,omitempty"`
}

type Daemon struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
	ConnectedAt  string   `json:"connectedAt"`
	LastSeen     string   `json:"lastSeen"`
}

type Message struct {
	ID         string   `json:"id"`
	ChannelID  string   `json:"channelId"`
	AuthorKind string   `json:"authorKind"`
	AuthorID   string   `json:"authorId"`
	AuthorName string   `json:"authorName"`
	Text       string   `json:"text"`
	Kind       string   `json:"kind"`
	Timestamp  string   `json:"timestamp"`
	Tags       []string `json:"tags,omitempty"`
	ProtocolID string   `json:"protocolId,omitempty"`
}

type State struct {
	Meta          Meta       `json:"meta"`
	CurrentUserID string     `json:"currentUserId"`
	Users         []User     `json:"users"`
	Channels      []Channel  `json:"channels"`
	Agents        []Agent    `json:"agents"`
	Daemons       []Daemon   `json:"daemons"`
	Messages      []Message  `json:"messages"`
	Events        []Envelope `json:"events"`
}

type DaemonHelloPayload struct {
	Token        string   `json:"token"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

type AgentSpawnPayload struct {
	Agent Agent `json:"agent"`
}

type AgentMessagePayload struct {
	Agent      Agent     `json:"agent"`
	Channel    Channel   `json:"channel"`
	Message    Message   `json:"message"`
	Recent     []Message `json:"recent"`
	PeerAgents []Agent   `json:"peerAgents,omitempty"`
}

type TaskAssignedPayload struct {
	Agent     Agent  `json:"agent"`
	ChannelID string `json:"channelId"`
	Task      string `json:"task"`
	MessageID string `json:"messageId"`
}

type AgentReplyPayload struct {
	AgentID   string   `json:"agentId"`
	ChannelID string   `json:"channelId"`
	Text      string   `json:"text"`
	Memory    []string `json:"memory,omitempty"`
}

type AgentStatusPayload struct {
	AgentID string `json:"agentId"`
	Status  string `json:"status"`
}

type MemoryUpsertPayload struct {
	AgentID string `json:"agentId"`
	Text    string `json:"text"`
	Source  string `json:"source"`
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func NewID(prefix string) string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(time.Now().Format(time.RFC3339Nano))))
	}
	return prefix + "_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
}

func Raw(payload any) json.RawMessage {
	if payload == nil {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{"error":"payload marshal failed"}`)
	}
	return b
}

func NewEnvelope(serverID, typ string, actor Actor, scope Scope, payload any, causationID string) Envelope {
	correlationID := causationID
	if correlationID == "" {
		correlationID = NewID("corr")
	}
	return Envelope{
		ID:       NewID("evt"),
		Type:     typ,
		TS:       Now(),
		ServerID: serverID,
		Actor:    actor,
		Scope:    scope,
		Payload:  Raw(payload),
		Trace: Trace{
			CorrelationID: correlationID,
			CausationID:   causationID,
		},
	}
}

func DecodePayload[T any](env Envelope) (T, error) {
	var out T
	err := json.Unmarshal(env.Payload, &out)
	return out, err
}

var mentionRE = regexp.MustCompile(`(?i)@([a-z0-9][a-z0-9_-]{0,40})`)

func ExtractMentions(text string, agents []Agent) []string {
	matches := mentionRE.FindAllStringSubmatch(strings.ToLower(text), -1)
	if len(matches) == 0 {
		return nil
	}

	wanted := make(map[string]bool)
	for _, m := range matches {
		wanted[m[1]] = true
	}

	var ids []string
	seen := make(map[string]bool)
	for _, agent := range agents {
		name := strings.ToLower(strings.ReplaceAll(agent.Name, " ", "-"))
		id := strings.ToLower(agent.ID)
		shortID := strings.TrimPrefix(id, "agent_")
		if wanted[name] || wanted[id] || wanted[shortID] {
			if !seen[agent.ID] {
				ids = append(ids, agent.ID)
				seen[agent.ID] = true
			}
		}
	}
	return ids
}

func TrimEvents(events []Envelope, limit int) []Envelope {
	if len(events) <= limit {
		return events
	}
	return append([]Envelope(nil), events[len(events)-limit:]...)
}

func TrimMessages(messages []Message, limit int) []Message {
	if len(messages) <= limit {
		return messages
	}
	return append([]Message(nil), messages[len(messages)-limit:]...)
}
