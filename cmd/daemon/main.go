package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/xargin/open-agent-room/internal/protocol"
	"github.com/xargin/open-agent-room/internal/websocket"
)

type memoryFile struct {
	Agents map[string][]string `json:"agents"`
}

type daemon struct {
	conn          *websocket.Conn
	serverID      string
	name          string
	memory        memoryFile
	memPath       string
	runner        string
	runnerFormat  string
	customRunner  bool
	forceDemo     bool
	runnerTimeout time.Duration
	runnerWorkdir string
}

type runnerRequest struct {
	EventType   string             `json:"eventType"`
	ServerID    string             `json:"serverId"`
	ChannelID   string             `json:"channelId"`
	Prompt      string             `json:"prompt"`
	Agent       protocol.Agent     `json:"agent"`
	Memories    []string           `json:"memories"`
	Recent      []protocol.Message `json:"recent"`
	CausationID string             `json:"causationId"`
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
	runnerFlag := flag.String("runner", getenv("OPEN_AGENT_RUNNER", "auto"), "local agent command, auto, or demo")
	runnerFormatFlag := flag.String("runner-format", getenv("OPEN_AGENT_RUNNER_FORMAT", "json"), "runner stdin format: json or prompt")
	runnerTimeoutFlag := flag.Duration("runner-timeout", getenvDuration("OPEN_AGENT_RUNNER_TIMEOUT", 2*time.Minute), "local agent command timeout")
	runnerWorkdirFlag := flag.String("runner-workdir", getenv("OPEN_AGENT_RUNNER_WORKDIR", "."), "working directory for the local agent command")
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

	d := &daemon{
		conn:          conn,
		name:          *nameFlag,
		memory:        mem,
		memPath:       memPath,
		runner:        strings.TrimSpace(*runnerFlag),
		runnerFormat:  strings.ToLower(strings.TrimSpace(*runnerFormatFlag)),
		runnerTimeout: *runnerTimeoutFlag,
		runnerWorkdir: *runnerWorkdirFlag,
	}
	d.configureRunner()
	if d.runnerFormat == "" {
		d.runnerFormat = "json"
	}
	capabilities := d.capabilities()
	hello := protocol.NewEnvelope("", "daemon.hello", protocol.Actor{Kind: "daemon", ID: "local", Name: d.name}, protocol.Scope{Kind: "server", ID: "pending"}, protocol.DaemonHelloPayload{
		Token:        *tokenFlag,
		Name:         d.name,
		Capabilities: capabilities,
	}, "")
	if err := conn.WriteJSON(hello); err != nil {
		log.Fatal(err)
	}

	log.Printf("daemon connected to %s as %s", *urlFlag, d.name)
	if d.forceDemo {
		log.Printf("agent runtime: demo fallback forced")
	} else if d.runner == "" {
		log.Printf("agent runtime: per-agent runtime selection enabled")
	} else {
		log.Printf("agent runtime: custom runner %q with %s stdin", d.runner, d.runnerFormat)
	}
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
		return d.reply("agent.message", payload.Agent, payload.Channel.ID, payload.Message.Text, payload.Recent, env.ID)
	case "task.assigned":
		payload, err := protocol.DecodePayload[protocol.TaskAssignedPayload](env)
		if err != nil {
			return err
		}
		return d.reply("task.assigned", payload.Agent, payload.ChannelID, payload.Task, nil, env.ID)
	case "error":
		log.Printf("server error: %s", string(env.Payload))
	default:
		log.Printf("ignored event: %s", env.Type)
	}
	return nil
}

func (d *daemon) reply(eventType string, agent protocol.Agent, channelID, prompt string, recent []protocol.Message, causationID string) error {
	d.ensureAgent(agent.ID)
	if err := d.sendStatus(agent.ID, "thinking", causationID); err != nil {
		return err
	}

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

	request := runnerRequest{
		EventType:   eventType,
		ServerID:    d.serverID,
		ChannelID:   channelID,
		Prompt:      prompt,
		Agent:       agent,
		Memories:    memories,
		Recent:      recent,
		CausationID: causationID,
	}
	reply, err := d.buildAgentReply(request)
	if err != nil {
		reply = fmt.Sprintf("Local runner failed: %v\n\nFalling back to demo runtime.\n\n%s", err, buildReply(agent, prompt, memories))
	}
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

func (d *daemon) buildAgentReply(request runnerRequest) (string, error) {
	if d.customRunner {
		return d.runExternalAgent(request, d.runner, d.runnerFormat)
	}
	if d.forceDemo {
		time.Sleep(450 * time.Millisecond)
		return buildReply(request.Agent, request.Prompt, request.Memories), nil
	}
	command, format, err := d.commandForAgent(request.Agent)
	if err != nil {
		if request.Agent.Runtime == "demo" {
			time.Sleep(450 * time.Millisecond)
			return buildReply(request.Agent, request.Prompt, request.Memories), nil
		}
		return "", err
	}
	if command == "" {
		time.Sleep(450 * time.Millisecond)
		return buildReply(request.Agent, request.Prompt, request.Memories), nil
	}
	return d.runExternalAgent(request, command, format)
}

func (d *daemon) runExternalAgent(request runnerRequest, command, format string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.runnerTimeout)
	defer cancel()

	cmd := shellCommand(ctx, command)
	cmd.Dir = d.runnerWorkdir
	cmd.Env = append(os.Environ(),
		"OPEN_AGENT_EVENT_TYPE="+request.EventType,
		"OPEN_AGENT_SERVER_ID="+request.ServerID,
		"OPEN_AGENT_CHANNEL_ID="+request.ChannelID,
		"OPEN_AGENT_ID="+request.Agent.ID,
		"OPEN_AGENT_NAME="+request.Agent.Name,
		"OPEN_AGENT_RUNTIME="+request.Agent.Runtime,
		"OPEN_AGENT_MODEL="+request.Agent.Model,
	)

	var stdin bytes.Buffer
	switch format {
	case "json":
		if err := json.NewEncoder(&stdin).Encode(request); err != nil {
			return "", err
		}
	case "prompt":
		stdin.WriteString(buildRunnerPrompt(request))
	default:
		return "", fmt.Errorf("unsupported runner format %q", format)
	}
	cmd.Stdin = &stdin

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	text := strings.TrimSpace(stdout.String())
	errText := strings.TrimSpace(stderr.String())
	if ctx.Err() == context.DeadlineExceeded {
		if text != "" {
			return "", fmt.Errorf("runner timed out after %s with output: %s", d.runnerTimeout, compact(text, 900))
		}
		if errText != "" {
			return "", fmt.Errorf("runner timed out after %s with stderr: %s", d.runnerTimeout, compact(errText, 900))
		}
		return "", fmt.Errorf("runner timed out after %s", d.runnerTimeout)
	}
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("%w: %s", err, compact(text, 900))
		}
		if errText != "" {
			return "", fmt.Errorf("%w: %s", err, compact(errText, 900))
		}
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("runner produced no stdout")
	}
	return text, nil
}

func (d *daemon) configureRunner() {
	switch strings.ToLower(strings.TrimSpace(d.runner)) {
	case "", "auto":
		d.runner = ""
		d.customRunner = false
		d.forceDemo = false
		d.runnerFormat = "prompt"
	case "demo", "none", "off":
		d.runner = ""
		d.customRunner = false
		d.forceDemo = true
		d.runnerFormat = "prompt"
	default:
		d.customRunner = true
		d.forceDemo = false
	}
}

func (d *daemon) capabilities() []string {
	capabilities := []string{"memory", "task-runner", "demo-agent"}
	if d.forceDemo {
		return capabilities
	}
	if _, err := exec.LookPath("codex"); err == nil {
		capabilities = append(capabilities, "codex")
	}
	if _, err := exec.LookPath("claude"); err == nil {
		capabilities = append(capabilities, "claude")
	}
	if d.customRunner {
		capabilities = append(capabilities, "external-runner")
	}
	return capabilities
}

func (d *daemon) commandForAgent(agent protocol.Agent) (string, string, error) {
	runtimeName := strings.ToLower(strings.TrimSpace(agent.Runtime))
	if runtimeName == "" {
		runtimeName = "codex"
	}
	switch runtimeName {
	case "demo":
		return "", "prompt", nil
	case "codex":
		path, err := exec.LookPath("codex")
		if err != nil {
			return "", "", fmt.Errorf("Codex runtime selected for %s, but codex CLI is not available", agent.Name)
		}
		return d.codexCommand(path, agent.Model), "prompt", nil
	case "claude":
		path, err := exec.LookPath("claude")
		if err != nil {
			return "", "", fmt.Errorf("Claude runtime selected for %s, but claude CLI is not available", agent.Name)
		}
		return d.claudeCommand(path, agent.Model), "prompt", nil
	default:
		return "", "", fmt.Errorf("unsupported runtime %q for %s", agent.Runtime, agent.Name)
	}
}

func (d *daemon) codexCommand(path, model string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s --ask-for-approval never --search exec -C %s --sandbox workspace-write --color never --ephemeral", shellQuote(path), shellQuote(d.runnerWorkdir))
	if strings.TrimSpace(model) != "" {
		fmt.Fprintf(&b, " -m %s", shellQuote(strings.TrimSpace(model)))
	}
	b.WriteString(" -")
	return b.String()
}

func (d *daemon) claudeCommand(path, model string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s -p --permission-mode acceptEdits --no-session-persistence --output-format text", shellQuote(path))
	if strings.TrimSpace(model) != "" {
		fmt.Fprintf(&b, " --model %s", shellQuote(strings.TrimSpace(model)))
	}
	return b.String()
}

func buildRunnerPrompt(request runnerRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, an agent in Open Agent Room.\n", request.Agent.Name)
	if request.Agent.Persona != "" {
		fmt.Fprintf(&b, "Persona: %s\n", request.Agent.Persona)
	}
	if request.Agent.Runtime != "" {
		fmt.Fprintf(&b, "Runtime: %s\n", request.Agent.Runtime)
	}
	if request.Agent.Model != "" {
		fmt.Fprintf(&b, "Model: %s\n", request.Agent.Model)
	}
	fmt.Fprintf(&b, "Event type: %s\n", request.EventType)
	fmt.Fprintf(&b, "Channel ID: %s\n\n", request.ChannelID)
	if len(request.Memories) > 0 {
		b.WriteString("Relevant memories:\n")
		for _, memory := range request.Memories {
			fmt.Fprintf(&b, "- %s\n", memory)
		}
		b.WriteString("\n")
	}
	if len(request.Recent) > 0 {
		b.WriteString("Recent channel context:\n")
		for _, message := range request.Recent {
			fmt.Fprintf(&b, "- %s: %s\n", message.AuthorName, compact(message.Text, 500))
		}
		b.WriteString("\n")
	}
	b.WriteString("Task:\n")
	b.WriteString(request.Prompt)
	b.WriteString("\n\nReturn only the message that should be posted back into the channel.")
	return b.String()
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "/bin/sh", "-lc", command)
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
	fmt.Fprintf(&b, "%s is running in demo fallback mode.\n\n", agent.Name)
	fmt.Fprintf(&b, "I received: %s\n\n", compact(prompt, 220))
	b.WriteString("No real local agent command is configured on this daemon. Start the daemon with `--runner auto` or set `OPEN_AGENT_RUNNER` to a local CLI agent command.")
	if len(memories) > 0 {
		latest := memories[len(memories)-1]
		fmt.Fprintf(&b, "\nMemory in scope: %s", compact(latest, 160))
	}
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

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
