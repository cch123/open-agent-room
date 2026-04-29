package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xargin/open-agent-room/internal/protocol"
	"github.com/xargin/open-agent-room/internal/websocket"
)

type memoryFile struct {
	Agents map[string][]string `json:"agents"`
}

type daemon struct {
	conn     *websocket.Conn
	serverID string
	name     string
	memory   memoryFile
	memPath  string
}

func main() {
	defaultURL := getenv("SLOCK_SERVER_URL", "ws://localhost:8787/daemon")
	defaultName, _ := os.Hostname()
	if defaultName == "" {
		defaultName = "local-daemon"
	}
	urlFlag := flag.String("url", defaultURL, "server websocket URL")
	nameFlag := flag.String("name", defaultName, "daemon display name")
	tokenFlag := flag.String("token", getenv("SLOCK_TOKEN", "dev-token"), "shared daemon token")
	flag.Parse()

	memPath := filepath.Join(getenv("SLOCK_DAEMON_HOME", ".openslock-daemon"), "memory.json")
	mem, err := loadMemory(memPath)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := websocket.Dial(*urlFlag)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	d := &daemon{conn: conn, name: *nameFlag, memory: mem, memPath: memPath}
	hello := protocol.NewEnvelope("", "daemon.hello", protocol.Actor{Kind: "daemon", ID: "local", Name: d.name}, protocol.Scope{Kind: "server", ID: "pending"}, protocol.DaemonHelloPayload{
		Token:        *tokenFlag,
		Name:         d.name,
		Capabilities: []string{"demo-agent", "memory", "task-runner"},
	}, "")
	if err := conn.WriteJSON(hello); err != nil {
		log.Fatal(err)
	}

	log.Printf("daemon connected to %s as %s", *urlFlag, d.name)
	for {
		var env protocol.Envelope
		if err := conn.ReadJSON(&env); err != nil {
			log.Fatal(err)
		}
		if err := d.handle(env); err != nil {
			log.Printf("handle %s: %v", env.Type, err)
		}
	}
}

func (d *daemon) handle(env protocol.Envelope) error {
	switch env.Type {
	case "daemon.ready":
		var payload struct {
			DaemonID string `json:"daemonId"`
			ServerID string `json:"serverId"`
		}
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return err
		}
		d.serverID = payload.ServerID
		log.Printf("registered as %s on %s", payload.DaemonID, payload.ServerID)
	case "agent.spawn":
		payload, err := protocol.DecodePayload[protocol.AgentSpawnPayload](env)
		if err != nil {
			return err
		}
		d.ensureAgent(payload.Agent.ID)
		log.Printf("agent ready: %s", payload.Agent.Name)
		return d.sendStatus(payload.Agent.ID, "idle", env.ID)
	case "agent.message":
		payload, err := protocol.DecodePayload[protocol.AgentMessagePayload](env)
		if err != nil {
			return err
		}
		return d.reply(payload.Agent, payload.Channel.ID, payload.Message.Text, env.ID)
	case "task.assigned":
		payload, err := protocol.DecodePayload[protocol.TaskAssignedPayload](env)
		if err != nil {
			return err
		}
		return d.reply(payload.Agent, payload.ChannelID, payload.Task, env.ID)
	case "error":
		log.Printf("server error: %s", string(env.Payload))
	default:
		log.Printf("ignored event: %s", env.Type)
	}
	return nil
}

func (d *daemon) reply(agent protocol.Agent, channelID, prompt, causationID string) error {
	d.ensureAgent(agent.ID)
	if err := d.sendStatus(agent.ID, "thinking", causationID); err != nil {
		return err
	}
	time.Sleep(450 * time.Millisecond)

	memories := d.memory.Agents[agent.ID]
	remembered := extractMemory(prompt)
	if remembered != "" {
		d.memory.Agents[agent.ID] = append(memories, remembered)
		if len(d.memory.Agents[agent.ID]) > 20 {
			d.memory.Agents[agent.ID] = d.memory.Agents[agent.ID][len(d.memory.Agents[agent.ID])-20:]
		}
		if err := saveMemory(d.memPath, d.memory); err != nil {
			return err
		}
		memEnv := d.newEnvelope("memory.upsert", protocol.Actor{Kind: "agent", ID: agent.ID, Name: agent.Name}, protocol.Scope{Kind: "channel", ID: channelID}, protocol.MemoryUpsertPayload{
			AgentID: agent.ID,
			Text:    remembered,
			Source:  "daemon",
		}, causationID)
		if err := d.conn.WriteJSON(memEnv); err != nil {
			return err
		}
		memories = d.memory.Agents[agent.ID]
	}

	reply := buildReply(agent, prompt, memories)
	replyEnv := d.newEnvelope("agent.reply", protocol.Actor{Kind: "agent", ID: agent.ID, Name: agent.Name}, protocol.Scope{Kind: "channel", ID: channelID}, protocol.AgentReplyPayload{
		AgentID:   agent.ID,
		ChannelID: channelID,
		Text:      reply,
	}, causationID)
	if err := d.conn.WriteJSON(replyEnv); err != nil {
		return err
	}
	return d.sendStatus(agent.ID, "idle", causationID)
}

func (d *daemon) sendStatus(agentID, status, causationID string) error {
	env := d.newEnvelope("agent.status", protocol.Actor{Kind: "daemon", ID: "local", Name: d.name}, protocol.Scope{Kind: "server", ID: d.serverID}, protocol.AgentStatusPayload{
		AgentID: agentID,
		Status:  status,
	}, causationID)
	return d.conn.WriteJSON(env)
}

func (d *daemon) newEnvelope(typ string, actor protocol.Actor, scope protocol.Scope, payload any, causationID string) protocol.Envelope {
	if scope.ID == "" {
		scope.ID = d.serverID
	}
	return protocol.NewEnvelope(d.serverID, typ, actor, scope, payload, causationID)
}

func (d *daemon) ensureAgent(agentID string) {
	if d.memory.Agents == nil {
		d.memory.Agents = make(map[string][]string)
	}
	if _, ok := d.memory.Agents[agentID]; !ok {
		d.memory.Agents[agentID] = nil
		_ = saveMemory(d.memPath, d.memory)
	}
}

func buildReply(agent protocol.Agent, prompt string, memories []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s here. I received the request and can take the next step.\n\n", agent.Name)
	fmt.Fprintf(&b, "Working read: %s\n\n", compact(prompt, 220))
	b.WriteString("Next action:\n")
	b.WriteString("1. Identify the concrete deliverable.\n")
	b.WriteString("2. Split it into the smallest useful task.\n")
	b.WriteString("3. Send progress back through this same channel.\n")
	if len(memories) > 0 {
		latest := memories[len(memories)-1]
		fmt.Fprintf(&b, "\nMemory in scope: %s", compact(latest, 160))
	}
	b.WriteString("\n\nProtocol: handled by local daemon over `agent.message`/`task.assigned` -> `agent.reply`.")
	return b.String()
}

func compact(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= limit {
		return text
	}
	return text[:limit-1] + "..."
}

func extractMemory(text string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, "remember:")
	if idx == -1 {
		return ""
	}
	return strings.TrimSpace(text[idx+len("remember:"):])
}

func loadMemory(path string) (memoryFile, error) {
	var mem memoryFile
	mem.Agents = make(map[string][]string)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return mem, nil
	}
	if err != nil {
		return mem, err
	}
	if err := json.Unmarshal(b, &mem); err != nil {
		return mem, err
	}
	if mem.Agents == nil {
		mem.Agents = make(map[string][]string)
	}
	return mem, nil
}

func saveMemory(path string, mem memoryFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
