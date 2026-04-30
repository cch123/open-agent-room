package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/xargin/open-agent-room/internal/protocol"
	"github.com/xargin/open-agent-room/internal/realtime"
	"github.com/xargin/open-agent-room/internal/store"
	"github.com/xargin/open-agent-room/internal/websocket"
	"github.com/xargin/open-agent-room/internal/webui"
)

const maxAgentThreadDepth = 6

type app struct {
	store        *store.Store
	hub          *realtime.Hub
	daemons      *daemonRegistry
	token        string
	activeMu     sync.RWMutex
	activeAgents map[string]string
}

type daemonClient struct {
	id      string
	name    string
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (c *daemonClient) writeJSON(env protocol.Envelope) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(env)
}

type daemonRegistry struct {
	mu      sync.RWMutex
	clients map[string]*daemonClient
}

func newDaemonRegistry() *daemonRegistry {
	return &daemonRegistry{clients: make(map[string]*daemonClient)}
}

func (d *daemonRegistry) add(client *daemonClient) {
	d.mu.Lock()
	d.clients[client.id] = client
	d.mu.Unlock()
}

func (d *daemonRegistry) remove(id string) {
	d.mu.Lock()
	delete(d.clients, id)
	d.mu.Unlock()
}

func (d *daemonRegistry) first() *daemonClient {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, client := range d.clients {
		return client
	}
	return nil
}

func (d *daemonRegistry) get(id string) *daemonClient {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.clients[id]
}

func main() {
	dataPath := getenv("DATA_PATH", "data/state.json")
	st, err := store.New(dataPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := st.ResetRuntimePresence(); err != nil {
		log.Fatal(err)
	}
	a := &app{
		store:        st,
		hub:          realtime.NewHub(),
		daemons:      newDaemonRegistry(),
		token:        getenv("SLOCK_TOKEN", "dev-token"),
		activeAgents: make(map[string]string),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/events", a.handleEvents)
	mux.HandleFunc("/api/messages", a.handleMessages)
	mux.HandleFunc("/api/channels", a.handleChannels)
	mux.HandleFunc("/api/channels/", a.handleChannelSubroutes)
	mux.HandleFunc("/api/users", a.handleUsers)
	mux.HandleFunc("/api/users/", a.handleUserSubroutes)
	mux.HandleFunc("/api/skills", a.handleSkills)
	mux.HandleFunc("/api/skills/", a.handleSkillSubroutes)
	mux.HandleFunc("/api/agents", a.handleAgents)
	mux.HandleFunc("/api/agents/", a.handleAgentSubroutes)
	mux.HandleFunc("/daemon", a.handleDaemon)
	mux.Handle("/", webui.Handler())

	addr := ":" + getenv("PORT", "8787")
	log.Printf("Open Agent Room listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, logRequests(mux)))
}

func (a *app) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, a.store.Snapshot())
}

func (a *app) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := a.hub.Subscribe()
	defer a.hub.Unsubscribe(ch)

	_ = realtime.WriteEvent(w, realtime.Event{Name: "snapshot", Data: a.store.Snapshot()})
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			if err := realtime.WriteEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (a *app) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		ChannelID string `json:"channelId"`
		Text      string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, errors.New("message text is required"))
		return
	}
	if req.ChannelID == "" {
		req.ChannelID = "chan_general"
	}
	ch, ok := a.store.FindChannel(req.ChannelID)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("channel not found"))
		return
	}

	env := protocol.NewEnvelope(a.store.ServerID(), "message.created", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "channel", ID: ch.ID}, map[string]string{"text": req.Text}, "")
	msg := protocol.Message{
		ChannelID:  ch.ID,
		AuthorKind: "human",
		AuthorID:   "usr_you",
		AuthorName: "You",
		Text:       req.Text,
		Kind:       "message",
	}
	stored, err := a.store.AddMessage(msg, env)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.broadcast()

	if strings.HasPrefix(req.Text, "/assign ") {
		a.activateAssignTarget(ch.ID, stored.Text)
		go a.assignFromCommand(ch.ID, stored, env.ID)
	} else {
		snapshot := a.store.Snapshot()
		agentIDs := a.resolveAgentRoutes(ch, stored.Text, snapshot.Agents)
		go a.routeAgentMessages(ch, stored, agentIDs, env.ID)
	}

	writeJSON(w, http.StatusCreated, stored)
}

func (a *app) handleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Name  string `json:"name"`
		Topic string `json:"topic"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ch, err := a.store.AddChannel(req.Name, req.Topic)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "channel.created", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, ch, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	writeJSON(w, http.StatusCreated, ch)
}

func (a *app) handleChannelSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/channels/"), "/")
	if r.Method == http.MethodDelete {
		if path == "" || strings.Contains(path, "/") {
			http.NotFound(w, r)
			return
		}
		ch, err := a.store.DeleteChannel(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		a.clearActiveChannel(ch.ID)
		env := protocol.NewEnvelope(a.store.ServerID(), "channel.deleted", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, ch, "")
		_ = a.store.AddEnvelope(env)
		a.broadcast()
		writeJSON(w, http.StatusOK, ch)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !strings.HasSuffix(path, "/default-agent") {
		http.NotFound(w, r)
		return
	}
	channelID := strings.TrimSuffix(path, "/default-agent")
	channelID = strings.TrimSuffix(channelID, "/")
	var req struct {
		AgentID string `json:"agentId"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ch, err := a.store.UpdateChannelDefaultAgent(channelID, req.AgentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.setActiveAgent(ch.ID, ch.DefaultAgentID)
	env := protocol.NewEnvelope(a.store.ServerID(), "channel.default_agent.updated", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "channel", ID: ch.ID}, ch, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	writeJSON(w, http.StatusOK, ch)
}

func (a *app) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := a.store.AddUser(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "user.created", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, user, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	writeJSON(w, http.StatusCreated, user)
}

func (a *app) handleUserSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/users/"), "/")
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if path == "" || strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}
	user, err := a.store.DeleteUser(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "user.deleted", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, user, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	writeJSON(w, http.StatusOK, user)
}

func (a *app) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Name    string   `json:"name"`
		Source  string   `json:"source"`
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	skill, err := a.store.AddSkill(req.Name, req.Source, req.Content, req.Tags)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "skill.created", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, skill, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	writeJSON(w, http.StatusCreated, skill)
}

func (a *app) handleSkillSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/skills/"), "/")
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if path == "" || strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}
	skill, err := a.store.DeleteSkill(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "skill.deleted", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, skill, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	writeJSON(w, http.StatusOK, skill)
}

func (a *app) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Name         string                `json:"name"`
		Persona      string                `json:"persona"`
		SystemPrompt string                `json:"systemPrompt"`
		Runtime      string                `json:"runtime"`
		Model        string                `json:"model"`
		Skills       []protocol.AgentSkill `json:"skills"`
		SkillIDs     []string              `json:"skillIds"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	agent, err := a.store.AddAgent(req.Name, req.Persona, req.SystemPrompt, req.Runtime, req.Model, req.Skills, req.SkillIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "agent.created", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, agent, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	a.spawnAgent(agent)
	writeJSON(w, http.StatusCreated, agent)
}

func (a *app) handleAgentSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/agents/"), "/")
	if r.Method == http.MethodDelete {
		if agentID, skillID, ok := parseAgentSkillDeletePath(path); ok {
			skill, err := a.store.DeleteAgentSkill(agentID, skillID)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			env := protocol.NewEnvelope(a.store.ServerID(), "agent.skill.detached", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, map[string]any{"agentId": agentID, "skill": skill}, "")
			_ = a.store.AddEnvelope(env)
			a.broadcast()
			writeJSON(w, http.StatusOK, skill)
			return
		}
		if path == "" || strings.Contains(path, "/") {
			http.NotFound(w, r)
			return
		}
		agent, err := a.store.DeleteAgent(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		a.clearActiveAgentEverywhere(agent.ID)
		env := protocol.NewEnvelope(a.store.ServerID(), "agent.deleted", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, agent, "")
		_ = a.store.AddEnvelope(env)
		a.broadcast()
		writeJSON(w, http.StatusOK, agent)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if agentID, ok := parseAgentSkillPostPath(path); ok {
		var req struct {
			SkillID string   `json:"skillId"`
			Name    string   `json:"name"`
			Source  string   `json:"source"`
			Content string   `json:"content"`
			Tags    []string `json:"tags"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var (
			skill protocol.AgentSkill
			err   error
		)
		eventType := "agent.skill.attached"
		if strings.TrimSpace(req.SkillID) != "" {
			skill, err = a.store.AttachAgentSkill(agentID, req.SkillID)
		} else {
			skill, err = a.store.AddAgentSkill(agentID, req.Name, req.Source, req.Content, req.Tags)
			eventType = "agent.skill.created_and_attached"
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		env := protocol.NewEnvelope(a.store.ServerID(), eventType, protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, map[string]any{"agentId": agentID, "skill": skill}, "")
		_ = a.store.AddEnvelope(env)
		a.broadcast()
		writeJSON(w, http.StatusCreated, skill)
		return
	}
	if !strings.HasSuffix(path, "/assign") {
		http.NotFound(w, r)
		return
	}
	agentID := strings.TrimSuffix(path, "/assign")
	var req struct {
		ChannelID string `json:"channelId"`
		Task      string `json:"task"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	agent, ok := a.store.FindAgent(agentID)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("agent not found"))
		return
	}
	if req.ChannelID == "" {
		req.ChannelID = "chan_general"
	}
	if strings.TrimSpace(req.Task) == "" {
		writeError(w, http.StatusBadRequest, errors.New("task is required"))
		return
	}
	msg, err := a.assignTask(agent, req.ChannelID, req.Task, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

func parseAgentSkillPostPath(path string) (string, bool) {
	agentID, ok := strings.CutSuffix(path, "/skills")
	agentID = strings.Trim(agentID, "/")
	return agentID, ok && agentID != "" && !strings.Contains(agentID, "/")
}

func parseAgentSkillDeletePath(path string) (string, string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "skills" || parts[2] == "" {
		return "", "", false
	}
	return parts[0], parts[2], true
}

func (a *app) handleDaemon(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()

	var hello protocol.Envelope
	if err := conn.ReadJSON(&hello); err != nil {
		return
	}
	hello = a.normalizeEnvelope(hello)
	if hello.Type != "daemon.hello" {
		_ = conn.WriteJSON(a.errorEnvelope("expected daemon.hello", hello.ID))
		return
	}
	payload, err := protocol.DecodePayload[protocol.DaemonHelloPayload](hello)
	if err != nil || payload.Token != a.token {
		_ = conn.WriteJSON(a.errorEnvelope("daemon authentication failed", hello.ID))
		return
	}
	if payload.Name == "" {
		payload.Name = "local-daemon"
	}

	daemonID := protocol.NewID("daemon")
	now := protocol.Now()
	daemon := protocol.Daemon{
		ID:           daemonID,
		Name:         payload.Name,
		Status:       "online",
		Capabilities: payload.Capabilities,
		ConnectedAt:  now,
		LastSeen:     now,
	}
	_ = a.store.UpsertDaemon(daemon)
	_ = a.store.AddEnvelope(hello)
	client := &daemonClient{id: daemonID, name: daemon.Name, conn: conn}
	a.daemons.add(client)
	defer func() {
		a.daemons.remove(daemonID)
		_ = a.store.DisconnectDaemon(daemonID)
		a.broadcast()
	}()

	ready := protocol.NewEnvelope(a.store.ServerID(), "daemon.ready", protocol.Actor{Kind: "system", ID: "system"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, map[string]string{"daemonId": daemonID, "serverId": a.store.ServerID()}, hello.ID)
	_ = client.writeJSON(ready)
	a.broadcast()

	for _, agent := range a.store.Snapshot().Agents {
		a.spawnAgentOnClient(client, agent)
	}

	for {
		var env protocol.Envelope
		if err := conn.ReadJSON(&env); err != nil {
			return
		}
		env = a.normalizeEnvelope(env)
		a.handleDaemonEvent(daemonID, env)
	}
}

func (a *app) assignFromCommand(channelID string, msg protocol.Message, causationID string) {
	rest := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/assign"))
	parts := strings.Fields(rest)
	if len(parts) < 2 {
		return
	}
	agent, ok := a.store.FindAgent(parts[0])
	if !ok {
		return
	}
	task := strings.TrimSpace(strings.TrimPrefix(rest, parts[0]))
	_, _ = a.assignTask(agent, channelID, task, causationID)
}

func (a *app) activateAssignTarget(channelID, text string) {
	rest := strings.TrimSpace(strings.TrimPrefix(text, "/assign"))
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return
	}
	agent, ok := a.store.FindAgent(parts[0])
	if !ok {
		return
	}
	a.setActiveAgent(channelID, agent.ID)
}

func (a *app) assignTask(agent protocol.Agent, channelID, task, causationID string) (protocol.Message, error) {
	ch, ok := a.store.FindChannel(channelID)
	if !ok {
		return protocol.Message{}, errors.New("channel not found")
	}
	a.setActiveAgent(ch.ID, agent.ID)
	payload := protocol.TaskAssignedPayload{
		Agent:     agent,
		ChannelID: ch.ID,
		Task:      strings.TrimSpace(task),
		MessageID: protocol.NewID("task"),
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "task.assigned", protocol.Actor{Kind: "system", ID: "system"}, protocol.Scope{Kind: "channel", ID: ch.ID}, payload, causationID)
	msg := protocol.Message{
		ChannelID:  ch.ID,
		AuthorKind: "system",
		AuthorID:   "system",
		AuthorName: "Task Router",
		Text:       "Assigned to @" + agent.Name + ": " + payload.Task,
		Kind:       "task",
		Tags:       []string{"task", agent.ID},
	}
	stored, err := a.store.AddMessage(msg, env)
	if err != nil {
		return protocol.Message{}, err
	}
	a.broadcast()
	a.routeTask(agent, ch, payload, env)
	return stored, nil
}

func (a *app) routeAgentMessages(ch protocol.Channel, msg protocol.Message, agentIDs []string, causationID string) {
	a.routeAgentMessagesWithContext(ch, msg, agentIDs, nil, causationID, 0)
}

func (a *app) routeAgentMessagesWithContext(ch protocol.Channel, msg protocol.Message, agentIDs []string, peerPool []protocol.Agent, causationID string, threadDepth int) {
	agents := make([]protocol.Agent, 0, len(agentIDs))
	for _, id := range agentIDs {
		agent, ok := a.store.FindAgent(id)
		if !ok {
			a.clearActiveAgent(ch.ID, id)
			continue
		}
		agents = append(agents, agent)
	}
	if len(agents) == 0 {
		return
	}
	if len(peerPool) == 0 {
		peerPool = agents
	} else {
		peerPool = mergeAgents(peerPool, agents)
	}

	recent := a.store.RecentMessages(ch.ID, 12)
	for _, agent := range agents {
		payload := protocol.AgentMessagePayload{
			Agent:       agent,
			Channel:     ch,
			Message:     msg,
			Recent:      recent,
			PeerAgents:  peerAgentsFor(peerPool, agent.ID),
			ThreadDepth: threadDepth,
		}
		env := protocol.NewEnvelope(a.store.ServerID(), "agent.message", protocol.Actor{Kind: "system", ID: "router"}, protocol.Scope{Kind: "channel", ID: ch.ID}, payload, causationID)
		if !a.sendToAgent(agent, env) {
			a.fallbackReply(agent, ch.ID, msg.Text, env.ID)
		}
	}
}

func (a *app) resolveAgentRoutes(ch protocol.Channel, text string, agents []protocol.Agent) []string {
	agentIDs := protocol.ExtractMentions(text, agents)
	if len(agentIDs) == 1 {
		a.setActiveAgent(ch.ID, agentIDs[0])
	} else if len(agentIDs) > 1 {
		a.clearActiveChannel(ch.ID)
	} else if len(agentIDs) == 0 && !strings.Contains(text, "@") {
		if agentID, ok := a.activeAgent(ch.ID); ok {
			agentIDs = []string{agentID}
		} else if agentID, ok := defaultAgentForChannel(ch, agents); ok {
			agentIDs = []string{agentID}
			a.setActiveAgent(ch.ID, agentID)
		}
	}
	return agentIDs
}

func peerAgentsFor(agents []protocol.Agent, agentID string) []protocol.Agent {
	peers := make([]protocol.Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.ID != agentID {
			peers = append(peers, agent)
		}
	}
	return peers
}

func mergeAgents(groups ...[]protocol.Agent) []protocol.Agent {
	var merged []protocol.Agent
	seen := make(map[string]bool)
	for _, group := range groups {
		for _, agent := range group {
			if agent.ID == "" || seen[agent.ID] {
				continue
			}
			merged = append(merged, agent)
			seen[agent.ID] = true
		}
	}
	return merged
}

func routeTargetsFromAgentReply(text, authorAgentID string, agents []protocol.Agent) []string {
	agentIDs := protocol.ExtractMentions(text, agents)
	targets := make([]string, 0, len(agentIDs))
	seen := make(map[string]bool)
	for _, agentID := range agentIDs {
		if agentID == authorAgentID || seen[agentID] {
			continue
		}
		targets = append(targets, agentID)
		seen[agentID] = true
	}
	return targets
}

func routeTargetsForReplyPayload(payload protocol.AgentReplyPayload, authorAgentID string, agents []protocol.Agent) []string {
	targets := routeTargetsFromAgentReply(payload.Text, authorAgentID, agents)
	if len(targets) > 0 || mentionsCurrentUser(payload.Text) {
		return targets
	}
	seen := make(map[string]bool)
	for _, peer := range payload.PeerAgents {
		if peer.ID == "" || peer.ID == authorAgentID || seen[peer.ID] {
			continue
		}
		if _, ok := findAgentByID(agents, peer.ID); !ok {
			continue
		}
		targets = append(targets, peer.ID)
		seen[peer.ID] = true
	}
	return targets
}

func mentionsCurrentUser(text string) bool {
	return strings.Contains(strings.ToLower(text), "@you")
}

func findAgentByID(agents []protocol.Agent, agentID string) (protocol.Agent, bool) {
	for _, agent := range agents {
		if agent.ID == agentID {
			return agent, true
		}
	}
	return protocol.Agent{}, false
}

type agentMentionRoute struct {
	Channel     protocol.Channel
	Message     protocol.Message
	TargetIDs   []string
	PeerPool    []protocol.Agent
	CausationID string
	ThreadDepth int
}

func recentUnhandledAgentMentionRoutes(snapshot protocol.State, limit int) []agentMentionRoute {
	if limit <= 0 || len(snapshot.Messages) == 0 {
		return nil
	}

	handledReplies := make(map[string]bool)
	replyDepths := make(map[string]int)
	for _, env := range snapshot.Events {
		if env.Type == "agent.message" && env.Trace.CausationID != "" {
			handledReplies[env.Trace.CausationID] = true
			continue
		}
		if env.Type == "agent.reply" {
			payload, err := protocol.DecodePayload[protocol.AgentReplyPayload](env)
			if err == nil {
				replyDepths[env.ID] = payload.ThreadDepth
			}
		}
	}

	channels := make(map[string]protocol.Channel, len(snapshot.Channels))
	for _, ch := range snapshot.Channels {
		channels[ch.ID] = ch
	}
	agents := make(map[string]protocol.Agent, len(snapshot.Agents))
	for _, agent := range snapshot.Agents {
		agents[agent.ID] = agent
	}

	start := len(snapshot.Messages) - limit
	if start < 0 {
		start = 0
	}

	var routes []agentMentionRoute
	for _, msg := range snapshot.Messages[start:] {
		if msg.AuthorKind != "agent" || msg.ProtocolID == "" || handledReplies[msg.ProtocolID] {
			continue
		}
		threadDepth, ok := replyDepths[msg.ProtocolID]
		if !ok {
			continue
		}
		if threadDepth >= maxAgentThreadDepth {
			continue
		}
		author, ok := agents[msg.AuthorID]
		if !ok {
			continue
		}
		mentionedTargetIDs := routeTargetsFromAgentReply(msg.Text, author.ID, snapshot.Agents)
		if len(mentionedTargetIDs) == 0 {
			continue
		}
		targetAgents := make([]protocol.Agent, 0, len(mentionedTargetIDs))
		targetIDs := make([]string, 0, len(mentionedTargetIDs))
		for _, targetID := range mentionedTargetIDs {
			if target, ok := agents[targetID]; ok {
				targetAgents = append(targetAgents, target)
				targetIDs = append(targetIDs, target.ID)
			}
		}
		if len(targetAgents) == 0 {
			continue
		}
		ch, ok := channels[msg.ChannelID]
		if !ok {
			continue
		}
		routes = append(routes, agentMentionRoute{
			Channel:     ch,
			Message:     msg,
			TargetIDs:   append([]string(nil), targetIDs...),
			PeerPool:    mergeAgents([]protocol.Agent{author}, targetAgents),
			CausationID: msg.ProtocolID,
			ThreadDepth: threadDepth + 1,
		})
	}
	return routes
}

func (a *app) routeRecentUnhandledAgentMentions(limit int) {
	for _, route := range recentUnhandledAgentMentionRoutes(a.store.Snapshot(), limit) {
		a.routeAgentMessagesWithContext(route.Channel, route.Message, route.TargetIDs, route.PeerPool, route.CausationID, route.ThreadDepth)
	}
}

func defaultAgentForChannel(ch protocol.Channel, agents []protocol.Agent) (string, bool) {
	byID := make(map[string]protocol.Agent, len(agents))
	for _, agent := range agents {
		byID[agent.ID] = agent
	}
	if agent, ok := byID[ch.DefaultAgentID]; ok {
		return agent.ID, true
	}
	for _, memberID := range ch.MemberIDs {
		if agent, ok := byID[memberID]; ok {
			return agent.ID, true
		}
	}
	if len(agents) == 0 {
		return "", false
	}
	return agents[0].ID, true
}

func (a *app) activeAgent(channelID string) (string, bool) {
	a.activeMu.RLock()
	defer a.activeMu.RUnlock()
	agentID, ok := a.activeAgents[channelID]
	return agentID, ok && agentID != ""
}

func (a *app) setActiveAgent(channelID, agentID string) {
	if channelID == "" || agentID == "" {
		return
	}
	a.activeMu.Lock()
	if a.activeAgents == nil {
		a.activeAgents = make(map[string]string)
	}
	a.activeAgents[channelID] = agentID
	a.activeMu.Unlock()
}

func (a *app) clearActiveAgent(channelID, agentID string) {
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	if a.activeAgents[channelID] == agentID {
		delete(a.activeAgents, channelID)
	}
}

func (a *app) clearActiveChannel(channelID string) {
	a.activeMu.Lock()
	delete(a.activeAgents, channelID)
	a.activeMu.Unlock()
}

func (a *app) clearActiveAgentEverywhere(agentID string) {
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	for channelID, activeAgentID := range a.activeAgents {
		if activeAgentID == agentID {
			delete(a.activeAgents, channelID)
		}
	}
}

func (a *app) routeTask(agent protocol.Agent, ch protocol.Channel, payload protocol.TaskAssignedPayload, env protocol.Envelope) {
	if !a.sendToAgent(agent, env) {
		a.fallbackReply(agent, ch.ID, payload.Task, env.ID)
	}
}

func (a *app) sendToAgent(agent protocol.Agent, env protocol.Envelope) bool {
	client := a.daemons.get(agent.DaemonID)
	if client == nil {
		client = a.daemons.first()
	}
	if client == nil {
		return false
	}
	_ = a.store.UpdateAgentStatus(agent.ID, "thinking", client.id)
	a.broadcast()
	if err := client.writeJSON(env); err != nil {
		return false
	}
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	return true
}

func (a *app) spawnAgent(agent protocol.Agent) {
	client := a.daemons.first()
	if client == nil {
		return
	}
	a.spawnAgentOnClient(client, agent)
}

func (a *app) spawnAgentOnClient(client *daemonClient, agent protocol.Agent) {
	env := protocol.NewEnvelope(a.store.ServerID(), "agent.spawn", protocol.Actor{Kind: "system", ID: "system"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, protocol.AgentSpawnPayload{Agent: agent}, "")
	if err := client.writeJSON(env); err == nil {
		_ = a.store.AddEnvelope(env)
		_ = a.store.UpdateAgentStatus(agent.ID, "starting", client.id)
		a.broadcast()
	}
}

func (a *app) handleDaemonEvent(daemonID string, env protocol.Envelope) {
	switch env.Type {
	case "agent.ready":
		payload, err := protocol.DecodePayload[protocol.AgentStatusPayload](env)
		if err == nil {
			_ = a.store.UpdateAgentStatus(payload.AgentID, "idle", daemonID)
		}
		_ = a.store.AddEnvelope(env)
	case "agent.status":
		payload, err := protocol.DecodePayload[protocol.AgentStatusPayload](env)
		if err == nil {
			_ = a.store.UpdateAgentStatus(payload.AgentID, payload.Status, daemonID)
		}
		_ = a.store.AddEnvelope(env)
	case "agent.reply":
		payload, err := protocol.DecodePayload[protocol.AgentReplyPayload](env)
		if err == nil {
			a.appendAgentReply(payload, env, daemonID)
		}
	case "memory.upsert":
		payload, err := protocol.DecodePayload[protocol.MemoryUpsertPayload](env)
		if err == nil {
			_ = a.store.AppendMemory(payload.AgentID, payload.Text, payload.Source)
		}
		_ = a.store.AddEnvelope(env)
	default:
		_ = a.store.AddEnvelope(env)
	}
	a.broadcast()
}

func (a *app) appendAgentReply(payload protocol.AgentReplyPayload, env protocol.Envelope, daemonID string) {
	agent, ok := a.store.FindAgent(payload.AgentID)
	if !ok {
		return
	}
	msg := protocol.Message{
		ChannelID:  payload.ChannelID,
		AuthorKind: "agent",
		AuthorID:   agent.ID,
		AuthorName: agent.Name,
		Text:       payload.Text,
		Kind:       "message",
		Tags:       []string{"agent"},
	}
	stored, err := a.store.AddMessage(msg, env)
	if err != nil {
		return
	}
	for _, memory := range payload.Memory {
		_ = a.store.AppendMemory(agent.ID, memory, "reply")
	}
	_ = a.store.UpdateAgentStatus(agent.ID, "idle", daemonID)
	a.routeAgentReplyMentions(agent, stored, payload, env)
}

func (a *app) routeAgentReplyMentions(author protocol.Agent, msg protocol.Message, payload protocol.AgentReplyPayload, env protocol.Envelope) {
	if payload.ThreadDepth >= maxAgentThreadDepth {
		return
	}
	ch, ok := a.store.FindChannel(payload.ChannelID)
	if !ok {
		return
	}
	snapshot := a.store.Snapshot()
	targetIDs := routeTargetsForReplyPayload(payload, author.ID, snapshot.Agents)
	if len(targetIDs) == 0 {
		return
	}
	targetAgents := make([]protocol.Agent, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		if target, ok := a.store.FindAgent(targetID); ok {
			targetAgents = append(targetAgents, target)
		}
	}
	if len(targetAgents) == 0 {
		return
	}
	targetIDs = targetIDs[:0]
	for _, target := range targetAgents {
		targetIDs = append(targetIDs, target.ID)
	}
	peerPool := mergeAgents([]protocol.Agent{author}, targetAgents)
	go a.routeAgentMessagesWithContext(ch, msg, targetIDs, peerPool, env.ID, payload.ThreadDepth+1)
}

func (a *app) fallbackReply(agent protocol.Agent, channelID, prompt, causationID string) {
	_ = a.store.UpdateAgentStatus(agent.ID, "thinking", "")
	a.broadcast()
	time.Sleep(700 * time.Millisecond)

	memory := extractMemory(prompt)
	text := "I picked this up through the built-in runtime.\n\nPlan:\n1. Restate the requested outcome.\n2. Break it into a small next action.\n3. Report progress back in this channel.\n\nProtocol: no local daemon was attached, so the server handled this as `agent.reply`."
	if memory != "" {
		text += "\n\nMemory saved: " + memory
	}
	payload := protocol.AgentReplyPayload{AgentID: agent.ID, ChannelID: channelID, Text: text}
	if memory != "" {
		payload.Memory = []string{memory}
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "agent.reply", protocol.Actor{Kind: "agent", ID: agent.ID, Name: agent.Name}, protocol.Scope{Kind: "channel", ID: channelID}, payload, causationID)
	a.appendAgentReply(payload, env, "")
	a.broadcast()
}

func (a *app) normalizeEnvelope(env protocol.Envelope) protocol.Envelope {
	if env.ID == "" {
		env.ID = protocol.NewID("evt")
	}
	if env.TS == "" {
		env.TS = protocol.Now()
	}
	if env.ServerID == "" {
		env.ServerID = a.store.ServerID()
	}
	if env.Trace.CorrelationID == "" {
		env.Trace.CorrelationID = protocol.NewID("corr")
	}
	if len(env.Payload) == 0 {
		env.Payload = protocol.Raw(map[string]string{})
	}
	return env
}

func (a *app) errorEnvelope(message, causationID string) protocol.Envelope {
	return protocol.NewEnvelope(a.store.ServerID(), "error", protocol.Actor{Kind: "system", ID: "system"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, map[string]string{"message": message}, causationID)
}

func (a *app) broadcast() {
	a.hub.Publish("snapshot", a.store.Snapshot())
}

func extractMemory(text string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, "remember:")
	if idx == -1 {
		return ""
	}
	return strings.TrimSpace(text[idx+len("remember:"):])
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/events") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
