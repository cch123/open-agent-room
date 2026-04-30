package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/xargin/open-agent-room/internal/protocol"
	"github.com/xargin/open-agent-room/internal/websocket"
)

type memoryFile struct {
	Agents map[string][]string `json:"agents"`
}

type daemon struct {
	conn          *websocket.Conn
	writeMu       sync.Mutex
	serverID      string
	name          string
	memory        memoryFile
	memoryMu      sync.Mutex
	memPath       string
	runner        string
	runnerFormat  string
	customRunner  bool
	forceDemo     bool
	runnerTimeout time.Duration
	runnerWorkdir string
	activeMu      sync.Mutex
	activeRuns    map[string]activeRun
}

type activeRun struct {
	id        string
	channelID string
	cancel    context.CancelFunc
}

var errTaskCancelled = errors.New("task cancelled")

type runnerRequest struct {
	EventType   string             `json:"eventType"`
	ServerID    string             `json:"serverId"`
	ChannelID   string             `json:"channelId"`
	Workdir     string             `json:"workdir,omitempty"`
	Prompt      string             `json:"prompt"`
	Agent       protocol.Agent     `json:"agent"`
	PeerAgents  []protocol.Agent   `json:"peerAgents,omitempty"`
	ThreadDepth int                `json:"threadDepth,omitempty"`
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
		activeRuns:    make(map[string]activeRun),
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
	if err := d.writeEnvelope(hello); err != nil {
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
		d.startReply("agent.message", payload.Agent, payload.Channel.ID, "", payload.Message.Text, payload.PeerAgents, payload.ThreadDepth, payload.Recent, env.ID)
	case "task.assigned":
		payload, err := protocol.DecodePayload[protocol.TaskAssignedPayload](env)
		if err != nil {
			return err
		}
		d.startReply("task.assigned", payload.Agent, payload.ChannelID, payload.Workdir, payload.Task, nil, 0, nil, env.ID)
	case "task.updated":
		payload, err := protocol.DecodePayload[protocol.TaskOwnerEventPayload](env)
		if err != nil {
			return err
		}
		d.startReply("task.updated", payload.Agent, payload.ChannelID, payload.Task.Workdir, payload.Message, nil, 0, nil, env.ID)
	case "task.revoked":
		payload, err := protocol.DecodePayload[protocol.TaskOwnerEventPayload](env)
		if err != nil {
			return err
		}
		if d.cancelAgent(payload.Agent.ID, payload.ChannelID) {
			log.Printf("cancelled active task for %s after owner change", payload.Agent.Name)
		}
		return d.sendStatus(payload.Agent.ID, "idle", env.ID)
	case "error":
		log.Printf("server error: %s", string(env.Payload))
	default:
		log.Printf("ignored event: %s", env.Type)
	}
	return nil
}

func (d *daemon) startReply(eventType string, agent protocol.Agent, channelID, workdir, prompt string, peerAgents []protocol.Agent, threadDepth int, recent []protocol.Message, causationID string) {
	ctx, cancel := context.WithCancel(context.Background())
	runID := protocol.NewID("run")
	d.setActive(agent.ID, runID, channelID, cancel)
	go func() {
		defer d.clearActive(agent.ID, runID)
		log.Printf("agent %s handling %s", agent.Name, eventType)
		if err := d.reply(ctx, eventType, agent, channelID, workdir, prompt, peerAgents, threadDepth, recent, causationID); err != nil {
			if errors.Is(err, errTaskCancelled) {
				log.Printf("agent %s stopped %s", agent.Name, eventType)
				if statusErr := d.sendStatus(agent.ID, "idle", causationID); statusErr != nil {
					log.Printf("reset status for %s: %v", agent.Name, statusErr)
				}
				return
			}
			log.Printf("reply %s for %s: %v", eventType, agent.Name, err)
			if statusErr := d.sendStatus(agent.ID, "idle", causationID); statusErr != nil {
				log.Printf("reset status for %s: %v", agent.Name, statusErr)
			}
		}
	}()
}

func (d *daemon) reply(ctx context.Context, eventType string, agent protocol.Agent, channelID, workdir, prompt string, peerAgents []protocol.Agent, threadDepth int, recent []protocol.Message, causationID string) error {
	d.ensureAgent(agent.ID)
	if err := d.sendStatus(agent.ID, "thinking", causationID); err != nil {
		return err
	}

	memories, memEnv, err := d.prepareMemories(agent, channelID, prompt, causationID)
	if err != nil {
		return err
	}
	if memEnv != nil {
		if err := d.writeEnvelope(*memEnv); err != nil {
			return err
		}
	}

	request := runnerRequest{
		EventType:   eventType,
		ServerID:    d.serverID,
		ChannelID:   channelID,
		Workdir:     d.effectiveWorkdir(workdir),
		Prompt:      prompt,
		Agent:       agent,
		PeerAgents:  peerAgents,
		ThreadDepth: threadDepth,
		Memories:    memories,
		Recent:      recent,
		CausationID: causationID,
	}
	reply, err := d.buildAgentReply(ctx, request)
	if errors.Is(err, errTaskCancelled) {
		return err
	}
	if err != nil {
		reply = fmt.Sprintf("Local runner failed: %v\n\nFalling back to demo runtime.\n\n%s", err, buildReply(agent, prompt, memories, peerAgents))
	}
	replyEnv := d.newEnvelope("agent.reply", protocol.Actor{Kind: "agent", ID: agent.ID, Name: agent.Name}, protocol.Scope{Kind: "channel", ID: channelID}, protocol.AgentReplyPayload{
		AgentID:     agent.ID,
		ChannelID:   channelID,
		Text:        reply,
		PeerAgents:  peerAgents,
		ThreadDepth: threadDepth,
	}, causationID)
	if err := d.writeEnvelope(replyEnv); err != nil {
		return err
	}
	return d.sendStatus(agent.ID, "idle", causationID)
}

func (d *daemon) setActive(agentID, runID, channelID string, cancel context.CancelFunc) {
	d.activeMu.Lock()
	defer d.activeMu.Unlock()
	if d.activeRuns == nil {
		d.activeRuns = make(map[string]activeRun)
	}
	d.activeRuns[agentID] = activeRun{id: runID, channelID: channelID, cancel: cancel}
}

func (d *daemon) clearActive(agentID, runID string) {
	d.activeMu.Lock()
	defer d.activeMu.Unlock()
	if run, ok := d.activeRuns[agentID]; ok && run.id == runID {
		delete(d.activeRuns, agentID)
	}
}

func (d *daemon) cancelAgent(agentID, channelID string) bool {
	d.activeMu.Lock()
	run, ok := d.activeRuns[agentID]
	if ok && channelID != "" && run.channelID != channelID {
		ok = false
	}
	if ok {
		delete(d.activeRuns, agentID)
	}
	d.activeMu.Unlock()
	if ok {
		run.cancel()
	}
	return ok
}

func (d *daemon) prepareMemories(agent protocol.Agent, channelID, prompt, causationID string) ([]string, *protocol.Envelope, error) {
	remembered := extractMemory(prompt)

	d.memoryMu.Lock()
	defer d.memoryMu.Unlock()

	memories := append([]string(nil), d.memory.Agents[agent.ID]...)
	if remembered == "" {
		return memories, nil, nil
	}

	d.memory.Agents[agent.ID] = append(d.memory.Agents[agent.ID], remembered)
	if len(d.memory.Agents[agent.ID]) > 20 {
		d.memory.Agents[agent.ID] = d.memory.Agents[agent.ID][len(d.memory.Agents[agent.ID])-20:]
	}
	if err := saveMemory(d.memPath, d.memory); err != nil {
		return nil, nil, err
	}
	memories = append([]string(nil), d.memory.Agents[agent.ID]...)
	memEnv := d.newEnvelope("memory.upsert", protocol.Actor{Kind: "agent", ID: agent.ID, Name: agent.Name}, protocol.Scope{Kind: "channel", ID: channelID}, protocol.MemoryUpsertPayload{
		AgentID: agent.ID,
		Text:    remembered,
		Source:  "daemon",
	}, causationID)
	return memories, &memEnv, nil
}

func (d *daemon) buildAgentReply(ctx context.Context, request runnerRequest) (string, error) {
	if d.customRunner {
		return d.runExternalAgent(ctx, request, d.runner, d.runnerFormat)
	}
	if d.forceDemo {
		if err := sleepWithCancel(ctx, 450*time.Millisecond); err != nil {
			return "", err
		}
		return buildReply(request.Agent, request.Prompt, request.Memories, request.PeerAgents), nil
	}
	command, format, err := d.commandForAgent(request.Agent, request.Workdir)
	if err != nil {
		if request.Agent.Runtime == "demo" {
			if err := sleepWithCancel(ctx, 450*time.Millisecond); err != nil {
				return "", err
			}
			return buildReply(request.Agent, request.Prompt, request.Memories, request.PeerAgents), nil
		}
		return "", err
	}
	if command == "" {
		if err := sleepWithCancel(ctx, 450*time.Millisecond); err != nil {
			return "", err
		}
		return buildReply(request.Agent, request.Prompt, request.Memories, request.PeerAgents), nil
	}
	return d.runExternalAgent(ctx, request, command, format)
}

func sleepWithCancel(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return errTaskCancelled
	}
}

func (d *daemon) runExternalAgent(parent context.Context, request runnerRequest, command, format string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, d.runnerTimeout)
	defer cancel()

	cmd := shellCommand(ctx, command)
	cmd.Dir = request.Workdir
	cmd.Env = append(os.Environ(),
		"OPEN_AGENT_EVENT_TYPE="+request.EventType,
		"OPEN_AGENT_SERVER_ID="+request.ServerID,
		"OPEN_AGENT_CHANNEL_ID="+request.ChannelID,
		"OPEN_AGENT_WORKDIR="+request.Workdir,
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
	if errors.Is(ctx.Err(), context.Canceled) {
		return "", errTaskCancelled
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
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

func (d *daemon) effectiveWorkdir(workdir string) string {
	next := strings.TrimSpace(workdir)
	if next == "" {
		return d.runnerWorkdir
	}
	if filepath.IsAbs(next) {
		return next
	}
	return filepath.Join(d.runnerWorkdir, next)
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

func (d *daemon) commandForAgent(agent protocol.Agent, workdir string) (string, string, error) {
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
		return d.codexCommand(path, agent.Model, workdir), "prompt", nil
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

func (d *daemon) codexCommand(path, model, workdir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s --ask-for-approval never --search exec -C %s --sandbox workspace-write --color never --ephemeral", shellQuote(path), shellQuote(workdir))
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
	if request.Agent.SystemPrompt != "" {
		fmt.Fprintf(&b, "System prompt:\n%s\n", request.Agent.SystemPrompt)
	}
	if request.Agent.Runtime != "" {
		fmt.Fprintf(&b, "Runtime: %s\n", request.Agent.Runtime)
	}
	if request.Agent.Model != "" {
		fmt.Fprintf(&b, "Model: %s\n", request.Agent.Model)
	}
	fmt.Fprintf(&b, "Event type: %s\n", request.EventType)
	fmt.Fprintf(&b, "Channel ID: %s\n", request.ChannelID)
	if request.Workdir != "" {
		fmt.Fprintf(&b, "Working directory: %s\n", request.Workdir)
	}
	b.WriteString("\n")
	if request.EventType == "task.assigned" {
		b.WriteString("Task workflow:\n")
		b.WriteString("- You are the current owner of this task; the server has already moved it to Doing.\n")
		b.WriteString("- Use the working directory above as the task workspace.\n")
		b.WriteString("- Work in the task channel and keep progress concrete.\n")
		b.WriteString("- If this response finishes the requested work and it is ready for human or QA review, end with exactly: TASK_STATUS: review\n")
		b.WriteString("- Never mark the task Done yourself; Review is the handoff state.\n\n")
	}
	if request.EventType == "task.updated" {
		b.WriteString("Task update workflow:\n")
		b.WriteString("- You are still the owner of this task.\n")
		b.WriteString("- Treat this update as the latest source of truth and revise your execution plan if needed.\n")
		b.WriteString("- Use the working directory above as the task workspace.\n")
		b.WriteString("- If this response finishes the requested work and it is ready for human or QA review, end with exactly: TASK_STATUS: review\n")
		b.WriteString("- Never mark the task Done yourself; Review is the handoff state.\n\n")
	}
	if len(request.Memories) > 0 {
		b.WriteString("Relevant memories:\n")
		for _, memory := range request.Memories {
			fmt.Fprintf(&b, "- %s\n", memory)
		}
		b.WriteString("\n")
	}
	if len(request.Agent.Skills) > 0 {
		b.WriteString("Imported skills for this agent:\n")
		for _, skill := range request.Agent.Skills {
			fmt.Fprintf(&b, "## %s", skill.Name)
			if skill.Source != "" {
				fmt.Fprintf(&b, " (%s)", skill.Source)
			}
			if len(skill.Tags) > 0 {
				fmt.Fprintf(&b, " [%s]", strings.Join(skill.Tags, ", "))
			}
			b.WriteString("\n")
			b.WriteString(compact(skill.Content, 4000))
			b.WriteString("\n\n")
		}
	}
	if len(request.PeerAgents) > 0 {
		b.WriteString("Other agents addressed in the same user message:\n")
		for _, peer := range request.PeerAgents {
			fmt.Fprintf(&b, "- %s", protocol.MentionHandle(peer.Name))
			if peer.Persona != "" {
				fmt.Fprintf(&b, ": %s", peer.Persona)
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "\nCollaboration rule: This is turn %d of an agent-to-agent thread. During peer discussion, keep the reply short, concrete, and under 8 lines; do not paste the full proposal or long documentation yet. Use plain chat only during peer discussion: no Markdown headings, numbered document outlines, tables, code blocks, or document markers. If you still need peer input, explicitly mention the other participant using the exact @handle listed above and ask or answer them directly. Do not mention @You until the peer discussion has converged or you have a concrete final proposal. If the solution is settled, stop mentioning peer agents, provide only the final Markdown document between these exact markers, then mention @You after the end marker with a short handoff:\n<<<MARKDOWN_DOCUMENT>>>\n# Title\n...\n<<<END_MARKDOWN_DOCUMENT>>>\nAny handoff note, caveat, or @You message must be outside the markers, after <<<END_MARKDOWN_DOCUMENT>>>.\n\n", request.ThreadDepth+1)
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
	return strings.ToValidUTF8(b.String(), "\uFFFD")
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
	return d.writeEnvelope(env)
}

func (d *daemon) writeEnvelope(env protocol.Envelope) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return d.conn.WriteJSON(env)
}

func (d *daemon) newEnvelope(typ string, actor protocol.Actor, scope protocol.Scope, payload any, causationID string) protocol.Envelope {
	if scope.ID == "" {
		scope.ID = d.serverID
	}
	return protocol.NewEnvelope(d.serverID, typ, actor, scope, payload, causationID)
}

func (d *daemon) ensureAgent(agentID string) {
	d.memoryMu.Lock()
	defer d.memoryMu.Unlock()

	if d.memory.Agents == nil {
		d.memory.Agents = make(map[string][]string)
	}
	if _, ok := d.memory.Agents[agentID]; !ok {
		d.memory.Agents[agentID] = nil
		_ = saveMemory(d.memPath, d.memory)
	}
}

func buildReply(agent protocol.Agent, prompt string, memories []string, peerAgents []protocol.Agent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is running in demo fallback mode.\n\n", agent.Name)
	if len(peerAgents) > 0 {
		if line := peerMentionLine(peerAgents); line != "" {
			fmt.Fprintf(&b, "%s\n\n", line)
		}
	}
	fmt.Fprintf(&b, "I received: %s\n\n", compact(prompt, 220))
	b.WriteString("This fallback response was generated inside the daemon. If you expected a real local agent, start the daemon with `--runner auto` and check the selected CLI runtime.")
	if len(agent.Skills) > 0 {
		fmt.Fprintf(&b, "\nImported skills in scope: %d", len(agent.Skills))
	}
	if len(memories) > 0 {
		latest := memories[len(memories)-1]
		fmt.Fprintf(&b, "\nMemory in scope: %s", compact(latest, 160))
	}
	return b.String()
}

func peerMentionLine(peerAgents []protocol.Agent) string {
	var mentions []string
	for _, peer := range peerAgents {
		if strings.TrimSpace(peer.Name) != "" {
			mentions = append(mentions, protocol.MentionHandle(peer.Name))
		}
	}
	if len(mentions) == 0 {
		return ""
	}
	return "Looping in " + strings.Join(mentions, " ") + " for this multi-agent thread."
}

func compact(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	text = strings.ToValidUTF8(text, "\uFFFD")
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
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
