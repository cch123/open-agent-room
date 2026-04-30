package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/xargin/open-agent-room/internal/protocol"
	"github.com/xargin/open-agent-room/internal/realtime"
	"github.com/xargin/open-agent-room/internal/store"
	"github.com/xargin/open-agent-room/internal/websocket"
	"github.com/xargin/open-agent-room/internal/webui"
)

const (
	maxAgentThreadDepth       = 6
	maxRemoteSkillContentSize = 64 * 1024
	maxRemoteSkillPageSize    = 2 * 1024 * 1024
)

var skillImportHTTPClient = &http.Client{Timeout: 8 * time.Second}

var taskReviewMarkerRE = regexp.MustCompile(`(?im)^\s*TASK_STATUS\s*:\s*review\s*$`)

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
	mux.HandleFunc("/api/task-lanes", a.handleTaskLanes)
	mux.HandleFunc("/api/task-lanes/", a.handleTaskLaneSubroutes)
	mux.HandleFunc("/api/tasks", a.handleTasks)
	mux.HandleFunc("/api/tasks/", a.handleTaskSubroutes)
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
	snapshot := a.store.Snapshot()
	explicitAgentIDs := protocol.ExtractMentions(stored.Text, snapshot.Agents)
	if len(explicitAgentIDs) > 0 {
		a.assignTaskFromMention(ch.ID, explicitAgentIDs[0], env.ID)
	}
	a.broadcast()

	if strings.HasPrefix(req.Text, "/assign ") {
		a.activateAssignTarget(ch.ID, stored.Text)
		go a.assignFromCommand(ch.ID, stored, env.ID)
	} else {
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
		eventType := "channel.deleted"
		if ch.Hidden {
			eventType = "channel.hidden"
		}
		env := protocol.NewEnvelope(a.store.ServerID(), eventType, protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, ch, "")
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
	name, source, content, err := normalizeSkillImportRequest(r, req.Name, req.Source, req.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name, req.Source, req.Content = name, source, content
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

func (a *app) handleTaskLanes(w http.ResponseWriter, r *http.Request) {
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
	lane, err := a.store.AddTaskLane(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "task_lane.created", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, lane, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	writeJSON(w, http.StatusCreated, lane)
}

func (a *app) handleTaskLaneSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/task-lanes/"), "/")
	if path == "" || strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Position *int `json:"position"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.Position == nil {
			writeError(w, http.StatusBadRequest, errors.New("lane position is required"))
			return
		}
		lane, err := a.store.MoveTaskLane(path, *req.Position)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		env := protocol.NewEnvelope(a.store.ServerID(), "task_lane.moved", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, lane, "")
		_ = a.store.AddEnvelope(env)
		a.broadcast()
		writeJSON(w, http.StatusOK, lane)
	case http.MethodDelete:
		lane, err := a.store.DeleteTaskLane(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		env := protocol.NewEnvelope(a.store.ServerID(), "task_lane.deleted", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, lane, "")
		_ = a.store.AddEnvelope(env)
		a.broadcast()
		writeJSON(w, http.StatusOK, lane)
	default:
		methodNotAllowed(w)
	}
}

func (a *app) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Workdir     string `json:"workdir"`
		LaneID      string `json:"laneId"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	task, err := a.store.AddTask(req.Title, req.Description, req.Workdir, req.LaneID, "usr_you")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "task.created", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, task, "")
	_ = a.store.AddEnvelope(env)
	a.broadcast()
	writeJSON(w, http.StatusCreated, task)
}

func (a *app) handleTaskSubroutes(w http.ResponseWriter, r *http.Request) {
	taskPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/")
	if r.Method == http.MethodPost && strings.HasSuffix(taskPath, "/channel") {
		taskID := strings.Trim(strings.TrimSuffix(taskPath, "/channel"), "/")
		if taskID == "" || strings.Contains(taskID, "/") {
			http.NotFound(w, r)
			return
		}
		task, ch, created, err := a.store.CreateTaskChannel(taskID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		eventType := "task.channel.opened"
		if created {
			eventType = "task.channel.created"
		}
		payload := map[string]any{"task": task, "channel": ch, "created": created}
		env := protocol.NewEnvelope(a.store.ServerID(), eventType, protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "task", ID: task.ID}, payload, "")
		_ = a.store.AddEnvelope(env)
		a.broadcast()
		writeJSON(w, http.StatusOK, payload)
		return
	}
	if taskPath == "" || strings.Contains(taskPath, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Title        *string `json:"title"`
			Description  *string `json:"description"`
			Workdir      *string `json:"workdir"`
			LaneID       *string `json:"laneId"`
			AssigneeKind *string `json:"assigneeKind"`
			AssigneeID   *string `json:"assigneeId"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		before, beforeFound := taskByID(a.store.Snapshot().Tasks, taskPath)
		task, err := a.store.UpdateTask(taskPath, req.Title, req.Description, req.Workdir, req.LaneID, req.AssigneeKind, req.AssigneeID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		env := protocol.NewEnvelope(a.store.ServerID(), "task.updated", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "task", ID: task.ID}, task, "")
		_ = a.store.AddEnvelope(env)
		if beforeFound && taskAssigneeChanged(before, task) {
			a.revokePreviousTaskOwner(before, task, env.ID)
			if task.AssigneeKind != "" && task.AssigneeID != "" {
				task = a.startTaskForOwner(task, env.ID, true)
			}
		} else if task.AssigneeKind == "agent" && task.AssigneeID != "" {
			a.notifyTaskOwner(task, before, env.ID)
		}
		a.broadcast()
		writeJSON(w, http.StatusOK, task)
	case http.MethodDelete:
		task, err := a.store.DeleteTask(taskPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		env := protocol.NewEnvelope(a.store.ServerID(), "task.deleted", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, task, "")
		_ = a.store.AddEnvelope(env)
		a.broadcast()
		writeJSON(w, http.StatusOK, task)
	default:
		methodNotAllowed(w)
	}
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
	if r.Method == http.MethodPatch {
		if path == "" || strings.Contains(path, "/") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Name         *string `json:"name"`
			Persona      *string `json:"persona"`
			SystemPrompt *string `json:"systemPrompt"`
			Runtime      *string `json:"runtime"`
			Model        *string `json:"model"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		agent, err := a.store.UpdateAgent(path, req.Name, req.Persona, req.SystemPrompt, req.Runtime, req.Model)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		env := protocol.NewEnvelope(a.store.ServerID(), "agent.updated", protocol.Actor{Kind: "human", ID: "usr_you", Name: "You"}, protocol.Scope{Kind: "server", ID: a.store.ServerID()}, agent, "")
		_ = a.store.AddEnvelope(env)
		a.broadcast()
		a.spawnAgent(agent)
		writeJSON(w, http.StatusOK, agent)
		return
	}
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
			req.Name, req.Source, req.Content, err = normalizeSkillImportRequest(r, req.Name, req.Source, req.Content)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
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
	go a.routeRecentUnhandledAgentMentions(30)

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
		Text:       "Assigned to " + protocol.MentionHandle(agent.Name) + ": " + payload.Task,
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

func (a *app) assignTaskFromMention(channelID, agentID, causationID string) {
	task, changed, err := a.store.AssignTaskByChannel(channelID, "agent", agentID)
	if err != nil || !changed {
		return
	}
	task = a.startTaskProgress(task, causationID)
	payload := map[string]any{"task": task, "assigneeKind": "agent", "assigneeId": agentID, "source": "mention"}
	env := protocol.NewEnvelope(a.store.ServerID(), "task.assignee.changed", protocol.Actor{Kind: "system", ID: "router"}, protocol.Scope{Kind: "task", ID: task.ID}, payload, causationID)
	_ = a.store.AddEnvelope(env)
}

func (a *app) startTaskForOwner(task protocol.Task, causationID string, dispatch bool) protocol.Task {
	task = a.startTaskProgress(task, causationID)
	if dispatch {
		task = a.dispatchTaskToOwner(task, causationID)
	}
	return task
}

func (a *app) startTaskProgress(task protocol.Task, causationID string) protocol.Task {
	snapshot := a.store.Snapshot()
	if !strings.EqualFold(taskLaneName(snapshot.TaskLanes, task.LaneID), "Todo") {
		return task
	}
	moved, changed, err := a.store.MoveTaskToLaneName(task.ID, "Doing")
	if err != nil || !changed {
		return task
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "task.started", protocol.Actor{Kind: "system", ID: "router"}, protocol.Scope{Kind: "task", ID: moved.ID}, moved, causationID)
	_ = a.store.AddEnvelope(env)
	return moved
}

func (a *app) dispatchTaskToOwner(task protocol.Task, causationID string) protocol.Task {
	if task.AssigneeKind != "agent" || task.AssigneeID == "" {
		return task
	}
	agent, ok := a.store.FindAgent(task.AssigneeID)
	if !ok {
		return task
	}
	linkedTask, ch, _, err := a.store.CreateTaskChannel(task.ID)
	if err != nil {
		return task
	}
	task = linkedTask
	a.setActiveAgent(ch.ID, agent.ID)
	payload := protocol.TaskAssignedPayload{
		Agent:     agent,
		ChannelID: ch.ID,
		Task:      formatTaskAssignmentPrompt(task),
		Workdir:   task.Workdir,
		MessageID: protocol.NewID("task"),
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "task.assigned", protocol.Actor{Kind: "system", ID: "task-router"}, protocol.Scope{Kind: "channel", ID: ch.ID}, payload, causationID)
	go a.routeTask(agent, ch, payload, env)
	return task
}

func (a *app) notifyTaskOwner(task, before protocol.Task, causationID string) protocol.Task {
	if task.AssigneeKind != "agent" || task.AssigneeID == "" {
		return task
	}
	agent, ok := a.store.FindAgent(task.AssigneeID)
	if !ok {
		return task
	}
	linkedTask, ch, _, err := a.store.CreateTaskChannel(task.ID)
	if err != nil {
		return task
	}
	task = linkedTask
	payload := protocol.TaskOwnerEventPayload{
		Agent:             agent,
		ChannelID:         ch.ID,
		Task:              task,
		Message:           formatTaskUpdatePrompt(before, task),
		PreviousOwnerKind: before.AssigneeKind,
		PreviousOwnerID:   before.AssigneeID,
		CurrentOwnerKind:  task.AssigneeKind,
		CurrentOwnerID:    task.AssigneeID,
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "task.updated", protocol.Actor{Kind: "system", ID: "task-router"}, protocol.Scope{Kind: "channel", ID: ch.ID}, payload, causationID)
	if !a.sendToAgent(agent, env) {
		a.fallbackReply(agent, ch.ID, payload.Message, env.ID)
	}
	return task
}

func (a *app) revokePreviousTaskOwner(before, after protocol.Task, causationID string) {
	if before.AssigneeKind != "agent" || before.AssigneeID == "" || before.AssigneeID == after.AssigneeID {
		return
	}
	agent, ok := a.store.FindAgent(before.AssigneeID)
	if !ok {
		return
	}
	channelID := after.ChannelID
	if channelID == "" {
		channelID = before.ChannelID
	}
	if channelID == "" {
		linkedTask, ch, _, err := a.store.CreateTaskChannel(after.ID)
		if err != nil {
			return
		}
		after = linkedTask
		channelID = ch.ID
	}
	payload := protocol.TaskOwnerEventPayload{
		Agent:             agent,
		ChannelID:         channelID,
		Task:              after,
		Message:           formatTaskRevokedPrompt(before, after),
		PreviousOwnerKind: before.AssigneeKind,
		PreviousOwnerID:   before.AssigneeID,
		CurrentOwnerKind:  after.AssigneeKind,
		CurrentOwnerID:    after.AssigneeID,
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "task.revoked", protocol.Actor{Kind: "system", ID: "task-router"}, protocol.Scope{Kind: "channel", ID: channelID}, payload, causationID)
	if !a.sendControlToAgent(agent, env) {
		_ = a.store.AddEnvelope(env)
	}
}

func formatTaskAssignmentPrompt(task protocol.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are now the owner of this task.\n\nTitle: %s\n", task.Title)
	if strings.TrimSpace(task.Description) != "" {
		fmt.Fprintf(&b, "Description: %s\n", task.Description)
	}
	if strings.TrimSpace(task.Workdir) != "" {
		fmt.Fprintf(&b, "Working directory: %s\n", task.Workdir)
	}
	b.WriteString("\nWorkflow:\n")
	b.WriteString("- The server has moved this task to Doing.\n")
	b.WriteString("- Work in this task channel and report only concrete progress.\n")
	b.WriteString("- If you have finished the work and it is ready for human or QA review, end your reply with exactly: TASK_STATUS: review\n")
	b.WriteString("- Do not move the task to Done; Review is the handoff state.\n")
	return b.String()
}

func formatTaskUpdatePrompt(before, after protocol.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A task you own was updated.\n\nTitle: %s\n", after.Title)
	if strings.TrimSpace(after.Description) != "" {
		fmt.Fprintf(&b, "Description: %s\n", after.Description)
	}
	if strings.TrimSpace(after.Workdir) != "" {
		fmt.Fprintf(&b, "Working directory: %s\n", after.Workdir)
	}
	if before.Title != after.Title {
		fmt.Fprintf(&b, "Previous title: %s\n", before.Title)
	}
	if before.Description != after.Description {
		fmt.Fprintf(&b, "Previous description: %s\n", before.Description)
	}
	if before.Workdir != after.Workdir {
		fmt.Fprintf(&b, "Previous working directory: %s\n", valueOrNone(before.Workdir))
	}
	b.WriteString("\nUse this update as the latest task source of truth. Continue from the task channel, and if the work is ready for review, end with exactly: TASK_STATUS: review\n")
	return b.String()
}

func formatTaskRevokedPrompt(before, after protocol.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Stop working on this task immediately.\n\nTitle: %s\n", after.Title)
	if strings.TrimSpace(after.Description) != "" {
		fmt.Fprintf(&b, "Description: %s\n", after.Description)
	}
	if strings.TrimSpace(after.Workdir) != "" {
		fmt.Fprintf(&b, "Working directory: %s\n", after.Workdir)
	}
	if after.AssigneeKind != "" && after.AssigneeID != "" {
		fmt.Fprintf(&b, "\nThe task owner changed from %s/%s to %s/%s.", before.AssigneeKind, before.AssigneeID, after.AssigneeKind, after.AssigneeID)
	} else {
		fmt.Fprintf(&b, "\nThe task no longer has an owner. Your previous ownership %s/%s has been revoked.", before.AssigneeKind, before.AssigneeID)
	}
	b.WriteString("\nDo not continue execution or post a task completion for the revoked assignment.")
	return b.String()
}

func valueOrNone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(none)"
	}
	return value
}

func taskByID(tasks []protocol.Task, id string) (protocol.Task, bool) {
	id = strings.TrimSpace(id)
	for _, task := range tasks {
		if task.ID == id {
			return task, true
		}
	}
	return protocol.Task{}, false
}

func taskAssigneeChanged(before, after protocol.Task) bool {
	return before.AssigneeKind != after.AssigneeKind || before.AssigneeID != after.AssigneeID
}

func taskLaneName(lanes []protocol.TaskLane, laneID string) string {
	for _, lane := range lanes {
		if lane.ID == laneID {
			return lane.Name
		}
	}
	return ""
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
	threadStatus := protocol.NormalizeThreadStatus(payload.ThreadStatus)
	if threadStatus == "" {
		_, threadStatus = protocol.StripThreadStatusMarker(payload.Text)
	}
	if protocol.ThreadStatusStopsRouting(threadStatus) {
		return nil
	}
	targets := routeTargetsFromAgentReply(payload.Text, authorAgentID, agents)
	if len(targets) > 0 {
		if threadStatus == protocol.ThreadStatusContinue || agentReplyRequestsPeerTurn(payload.Text) {
			return targets
		}
		return nil
	}
	if mentionsCurrentUser(payload.Text) {
		return targets
	}
	if threadStatus != protocol.ThreadStatusContinue {
		return nil
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

func agentReplyRequestsPeerTurn(text string) bool {
	lower := strings.ToLower(text)
	if strings.ContainsAny(text, "?？") {
		return true
	}
	signals := []string{
		"请",
		"帮",
		"麻烦",
		"需要你",
		"能否",
		"可否",
		"是否",
		"你觉得",
		"怎么看",
		"确认一下",
		"最终确认",
		"can you",
		"could you",
		"please",
		"what do you",
		"do you",
		"wdyt",
		"review",
		"validate",
		"check",
	}
	for _, signal := range signals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
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

	handledReplyTargets := make(map[string]map[string]bool)
	replyDepths := make(map[string]int)
	replyPayloads := make(map[string]protocol.AgentReplyPayload)
	for _, env := range snapshot.Events {
		if env.Type == "agent.message" && env.Trace.CausationID != "" {
			payload, err := protocol.DecodePayload[protocol.AgentMessagePayload](env)
			if err == nil && payload.Agent.ID != "" {
				if handledReplyTargets[env.Trace.CausationID] == nil {
					handledReplyTargets[env.Trace.CausationID] = make(map[string]bool)
				}
				handledReplyTargets[env.Trace.CausationID][payload.Agent.ID] = true
			}
			continue
		}
		if env.Type == "agent.reply" {
			payload, err := protocol.DecodePayload[protocol.AgentReplyPayload](env)
			if err == nil {
				replyDepths[env.ID] = payload.ThreadDepth
				replyPayloads[env.ID] = payload
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
		if msg.AuthorKind != "agent" || msg.ProtocolID == "" {
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
		payload := replyPayloads[msg.ProtocolID]
		if payload.Text == "" {
			payload.Text = msg.Text
		}
		mentionedTargetIDs := routeTargetsForReplyPayload(payload, author.ID, snapshot.Agents)
		if len(mentionedTargetIDs) == 0 {
			continue
		}
		targetAgents := make([]protocol.Agent, 0, len(mentionedTargetIDs))
		targetIDs := make([]string, 0, len(mentionedTargetIDs))
		for _, targetID := range mentionedTargetIDs {
			if handledReplyTargets[msg.ProtocolID][targetID] {
				continue
			}
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

func (a *app) sendControlToAgent(agent protocol.Agent, env protocol.Envelope) bool {
	client := a.daemons.get(agent.DaemonID)
	if client == nil {
		client = a.daemons.first()
	}
	if client == nil {
		return false
	}
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
	cleanText, threadStatus := protocol.StripThreadStatusMarker(payload.Text)
	payload.Text = cleanText
	if payload.ThreadStatus == "" {
		payload.ThreadStatus = threadStatus
	}
	cleanText, explicitReview := stripTaskReviewMarker(payload.Text)
	payload.Text = cleanText
	env.Payload = protocol.Raw(payload)
	msg := protocol.Message{
		ChannelID:  payload.ChannelID,
		AuthorKind: "agent",
		AuthorID:   agent.ID,
		AuthorName: agent.Name,
		Text:       cleanText,
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
	a.advanceTaskToReviewForAgentReply(agent, payload.ChannelID, cleanText, explicitReview, env.ID)
	_ = a.store.UpdateAgentStatus(agent.ID, "idle", daemonID)
	a.routeAgentReplyMentions(agent, stored, payload, env)
}

func stripTaskReviewMarker(text string) (string, bool) {
	if !taskReviewMarkerRE.MatchString(text) {
		return text, false
	}
	clean := taskReviewMarkerRE.ReplaceAllString(text, "")
	clean = strings.TrimSpace(clean)
	return clean, true
}

func (a *app) advanceTaskToReviewForAgentReply(agent protocol.Agent, channelID, text string, explicit bool, causationID string) {
	task, ok := a.store.TaskForChannel(channelID)
	if !ok || task.AssigneeKind != "agent" || task.AssigneeID != agent.ID {
		return
	}
	if !explicit && !taskReplyLooksReadyForReview(text) {
		return
	}
	moved, changed, err := a.store.MoveTaskToLaneName(task.ID, "Review")
	if err != nil || !changed {
		return
	}
	env := protocol.NewEnvelope(a.store.ServerID(), "task.review_requested", protocol.Actor{Kind: "agent", ID: agent.ID, Name: agent.Name}, protocol.Scope{Kind: "task", ID: moved.ID}, moved, causationID)
	_ = a.store.AddEnvelope(env)
}

func taskReplyLooksReadyForReview(text string) bool {
	lower := strings.ToLower(text)
	blockers := []string{"not done", "not ready", "blocked", "未完成", "还没完成", "没有完成", "阻塞"}
	for _, blocker := range blockers {
		if strings.Contains(lower, blocker) {
			return false
		}
	}
	signals := []string{
		"ready for review",
		"ready to review",
		"completed",
		"finished",
		"implemented",
		"pr link",
		"pull request",
		"已完成",
		"完成了",
		"做完了",
		"可以 review",
		"进入 review",
		"可以验收",
		"准备验收",
		"可验收",
		"pr 链接",
	}
	for _, signal := range signals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
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

func normalizeSkillImportRequest(r *http.Request, name, source, content string) (string, string, string, error) {
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)
	source = strings.TrimSpace(source)
	if content == "" && source != "" {
		fetchURL, normalizedSource, sourceKind, err := resolveCloudSkillURL(source)
		if err != nil {
			return "", source, "", err
		}
		if fetchURL != "" {
			source = normalizedSource
			content, err = fetchRemoteSkillContent(r, fetchURL, sourceKind)
			if err != nil {
				return "", source, "", err
			}
		}
	}
	if name == "" && content != "" {
		name = deriveSkillName(source, content)
	}
	return name, source, content, nil
}

func fetchRemoteSkillContent(r *http.Request, source, sourceKind string) (string, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, source, nil)
	if err != nil {
		return "", fmt.Errorf("skill source URL is invalid: %w", err)
	}
	req.Header.Set("Accept", "text/markdown,text/plain,text/*;q=0.9,*/*;q=0.5")
	res, err := skillImportHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not fetch skill source: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("could not fetch skill source: HTTP %d", res.StatusCode)
	}
	limit := maxRemoteSkillContentSize
	if sourceKind == "skills.sh" {
		limit = maxRemoteSkillPageSize
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, int64(limit)+1))
	if err != nil {
		return "", fmt.Errorf("could not read skill source: %w", err)
	}
	if len(body) > limit {
		return "", errors.New("skill source content is too large")
	}
	content := strings.TrimSpace(string(body))
	if sourceKind == "skills.sh" {
		content, err = extractSkillsSHContent(content)
		if err != nil {
			return "", err
		}
	}
	if content == "" {
		return "", errors.New("skill source returned empty content")
	}
	if len([]byte(content)) > maxRemoteSkillContentSize {
		return "", errors.New("skill content is too large")
	}
	return content, nil
}

func resolveCloudSkillURL(raw string) (string, string, string, error) {
	source, err := extractCloudSkillSource(raw)
	if err != nil {
		return "", "", "", err
	}
	u, err := url.Parse(source)
	if err != nil {
		return "", source, "", fmt.Errorf("skill source URL is invalid: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", source, "", nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", source, "", errors.New("cloud import supports skills.sh links, GitHub links, and npx commands containing one of those links")
	}
	switch strings.ToLower(u.Hostname()) {
	case "skills.sh", "www.skills.sh":
		return u.String(), u.String(), "skills.sh", nil
	case "raw.githubusercontent.com":
		return u.String(), u.String(), "raw", nil
	case "github.com":
		rawURL, err := githubRawSkillURL(u)
		return rawURL, u.String(), "raw", err
	default:
		return "", source, "", errors.New("cloud import supports skills.sh links, GitHub links, and npx commands containing one of those links")
	}
}

var cloudSkillURLRE = regexp.MustCompile(`https?://[^\s"'<>]+`)

func extractCloudSkillSource(raw string) (string, error) {
	source := strings.TrimSpace(raw)
	if source == "" {
		return "", nil
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return strings.Trim(source, `"'`), nil
	}
	if !strings.HasPrefix(source, "npx ") && !strings.HasPrefix(source, "npx\t") && source != "npx" {
		return source, nil
	}
	for _, match := range cloudSkillURLRE.FindAllString(source, -1) {
		candidate := strings.TrimRight(strings.Trim(match, `"'`), ".,)")
		u, err := url.Parse(candidate)
		if err != nil || u.Scheme == "" || u.Host == "" {
			continue
		}
		switch strings.ToLower(u.Hostname()) {
		case "skills.sh", "www.skills.sh", "github.com", "raw.githubusercontent.com":
			return candidate, nil
		}
	}
	return "", errors.New("npx skill import command must contain a skills.sh or GitHub link")
}

func githubRawSkillURL(u *url.URL) (string, error) {
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	for i := range parts {
		value, err := url.PathUnescape(parts[i])
		if err == nil {
			parts[i] = value
		}
	}
	if len(parts) < 3 {
		return "", errors.New("GitHub URL must point to a file or skill directory")
	}
	owner, repo := parts[0], parts[1]
	switch parts[2] {
	case "blob", "raw":
		if len(parts) < 5 {
			return "", errors.New("GitHub file URL must include branch and file path")
		}
		ref := parts[3]
		filePath := strings.Join(parts[4:], "/")
		return "https://raw.githubusercontent.com/" + path.Join(owner, repo, ref, filePath), nil
	case "tree":
		if len(parts) < 5 {
			return "", errors.New("GitHub skill directory URL must include branch and directory path")
		}
		ref := parts[3]
		filePath := strings.Join(parts[4:], "/")
		if !strings.EqualFold(path.Base(filePath), "SKILL.md") {
			filePath = path.Join(filePath, "SKILL.md")
		}
		return "https://raw.githubusercontent.com/" + path.Join(owner, repo, ref, filePath), nil
	default:
		return "", errors.New("GitHub URL must be a blob file, raw file, or tree skill directory link")
	}
}

func extractSkillsSHContent(body string) (string, error) {
	if !looksLikeHTML(body) {
		return strings.TrimSpace(body), nil
	}
	start := strings.Index(body, ">SKILL.md<")
	if start != -1 {
		start += 1
	}
	if start == -1 {
		start = strings.Index(body, "SKILL.md")
	}
	if start == -1 {
		return "", errors.New("could not find SKILL.md content on skills.sh page")
	}
	section := body[start:]
	if script := strings.Index(section, "<script"); script >= 0 {
		section = section[:script]
	}
	text := htmlToMarkdownText(section)
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "SKILL.md"))
	if text == "" {
		return "", errors.New("could not extract SKILL.md content from skills.sh page")
	}
	return text, nil
}

func looksLikeHTML(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	return strings.HasPrefix(trimmed, "<!doctype html") || strings.HasPrefix(trimmed, "<html") || strings.Contains(trimmed, "<body")
}

func htmlToMarkdownText(value string) string {
	for _, noisyTag := range []string{"script", "style", "svg"} {
		value = regexp.MustCompile(`(?is)<`+noisyTag+`[^>]*>.*?</`+noisyTag+`>`).ReplaceAllString(value, "")
	}
	value = regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`).ReplaceAllStringFunc(value, func(match string) string {
		parts := regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`).FindStringSubmatch(match)
		if len(parts) != 2 {
			return "\n"
		}
		code := strings.Trim(htmlFragmentText(parts[1], true), "\n")
		if code == "" {
			return "\n"
		}
		return "\n\n```text\n" + code + "\n```\n\n"
	})
	replacements := []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?i)<h1[^>]*>`), "\n# "},
		{regexp.MustCompile(`(?i)</h1>`), "\n"},
		{regexp.MustCompile(`(?i)<h2[^>]*>`), "\n## "},
		{regexp.MustCompile(`(?i)</h2>`), "\n"},
		{regexp.MustCompile(`(?i)<h3[^>]*>`), "\n### "},
		{regexp.MustCompile(`(?i)</h3>`), "\n"},
		{regexp.MustCompile(`(?i)<li[^>]*>`), "\n- "},
		{regexp.MustCompile(`(?i)</li>`), "\n"},
		{regexp.MustCompile(`(?i)<p[^>]*>`), "\n\n"},
		{regexp.MustCompile(`(?i)</p>`), "\n"},
		{regexp.MustCompile(`(?i)<br\s*/?>`), "\n"},
		{regexp.MustCompile(`(?is)<code[^>]*>(.*?)</code>`), "`$1`"},
	}
	for _, replacement := range replacements {
		value = replacement.re.ReplaceAllString(value, replacement.repl)
	}
	value = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	inFence := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "```text" || strings.TrimSpace(line) == "```" {
			out = append(out, strings.TrimSpace(line))
			inFence = !inFence
			blank = false
			continue
		}
		if inFence {
			out = append(out, line)
			blank = false
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func htmlFragmentText(value string, preserveWhitespace bool) string {
	value = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(value, "\n")
	value = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	if preserveWhitespace {
		return strings.ReplaceAll(value, "\u00a0", " ")
	}
	return strings.TrimSpace(value)
}

func deriveSkillName(source, content string) string {
	if match := regexp.MustCompile(`(?m)^#\s+(.+)$`).FindStringSubmatch(content); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	u, err := url.Parse(strings.TrimSpace(source))
	if err == nil && u.Path != "" {
		base := path.Base(strings.Trim(u.Path, "/"))
		base = strings.TrimSuffix(base, path.Ext(base))
		if strings.EqualFold(base, "SKILL") {
			base = path.Base(path.Dir(strings.Trim(u.Path, "/")))
		}
		if base != "." && base != "/" && base != "" {
			return strings.TrimSpace(base)
		}
	}
	return "Imported skill"
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
