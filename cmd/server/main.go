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

type app struct {
	store   *store.Store
	hub     *realtime.Hub
	daemons *daemonRegistry
	token   string
}

type daemonClient struct {
	id   string
	name string
	conn *websocket.Conn
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
		store:   st,
		hub:     realtime.NewHub(),
		daemons: newDaemonRegistry(),
		token:   getenv("SLOCK_TOKEN", "dev-token"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/events", a.handleEvents)
	mux.HandleFunc("/api/messages", a.handleMessages)
	mux.HandleFunc("/api/channels", a.handleChannels)
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
		go a.assignFromCommand(ch.ID, stored, env.ID)
	} else {
		go a.routeMentions(ch, stored, env.ID)
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

func (a *app) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Name    string `json:"name"`
		Persona string `json:"persona"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	agent, err := a.store.AddAgent(req.Name, req.Persona)
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
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
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
	_ = conn.WriteJSON(ready)
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

func (a *app) assignTask(agent protocol.Agent, channelID, task, causationID string) (protocol.Message, error) {
	ch, ok := a.store.FindChannel(channelID)
	if !ok {
		return protocol.Message{}, errors.New("channel not found")
	}
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

func (a *app) routeMentions(ch protocol.Channel, msg protocol.Message, causationID string) {
	agentIDs := protocol.ExtractMentions(msg.Text, a.store.Snapshot().Agents)
	for _, id := range agentIDs {
		agent, ok := a.store.FindAgent(id)
		if !ok {
			continue
		}
		payload := protocol.AgentMessagePayload{
			Agent:   agent,
			Channel: ch,
			Message: msg,
			Recent:  a.store.RecentMessages(ch.ID, 12),
		}
		env := protocol.NewEnvelope(a.store.ServerID(), "agent.message", protocol.Actor{Kind: "system", ID: "router"}, protocol.Scope{Kind: "channel", ID: ch.ID}, payload, causationID)
		if !a.sendToAgent(agent, env) {
			a.fallbackReply(agent, ch.ID, msg.Text, env.ID)
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
	if err := client.conn.WriteJSON(env); err != nil {
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
	if err := client.conn.WriteJSON(env); err == nil {
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
	_, _ = a.store.AddMessage(msg, env)
	for _, memory := range payload.Memory {
		_ = a.store.AppendMemory(agent.ID, memory, "reply")
	}
	_ = a.store.UpdateAgentStatus(agent.ID, "idle", daemonID)
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
