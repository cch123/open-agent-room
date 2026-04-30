const themeStorageKey = "open-agent-room-theme";
const chatFontSizeStorageKey = "open-agent-room-chat-font-size";
const channelReadStorageKey = "open-agent-room-channel-read";

const state = {
  snapshot: null,
  view: "channel",
  channelId: "chan_general",
  theme: initialTheme(),
  chatFontSize: initialChatFontSize(),
  debugOpen: false,
  read: initialChannelReadState(),
  selectedEventId: null,
  mention: {
    active: false,
    start: -1,
    query: "",
    selected: 0,
  },
  skillAgentId: "",
  skillDialogMode: "agent",
  skillCreateMode: "local",
  skillSearch: "",
  skillTagFilter: "",
  taskDragId: "",
  focusTaskId: "",
  editingTaskId: "",
  taskDrawerTaskId: "",
  taskDrawerChannelId: "",
  taskDrawerDraft: "",
  agentStatusById: {},
  editingAgentId: "",
  channelManageMode: false,
  agentManageMode: false,
};

const markdownDocumentStartMarker = "<<<MARKDOWN_DOCUMENT>>>";
const markdownDocumentEndMarker = "<<<END_MARKDOWN_DOCUMENT>>>";
const markdownDocumentCardMinLines = 24;
const markdownDocumentCardMinChars = 1400;

const hoverTooltip = document.createElement("div");
hoverTooltip.className = "hover-tooltip";
hoverTooltip.hidden = true;
document.body.append(hoverTooltip);
let hoverTooltipTarget = null;

const profilePopover = document.createElement("div");
profilePopover.className = "profile-popover";
profilePopover.hidden = true;
document.body.append(profilePopover);
let profilePopoverTarget = null;

const runtimeModels = {
  codex: [
    ["", "CLI default"],
    ["gpt-5.3-codex", "GPT-5.3 Codex"],
    ["gpt-5.3-codex-spark", "GPT-5.3 Codex Spark"],
    ["gpt-5.4", "GPT-5.4"],
    ["gpt-5.4-mini", "GPT-5.4 Mini"],
    ["__custom", "Custom..."],
  ],
  claude: [
    ["", "CLI default"],
    ["sonnet", "Sonnet"],
    ["opus", "Opus"],
    ["claude-sonnet-4-6", "Claude Sonnet 4.6"],
    ["claude-opus-4-7", "Claude Opus 4.7"],
    ["__custom", "Custom..."],
  ],
  demo: [["", "No model"]],
};

const els = {
  appShell: document.querySelector(".app-shell"),
  channelList: document.querySelector("#channel-list"),
  userList: document.querySelector("#user-list"),
  agentList: document.querySelector("#agent-list"),
  openTasks: document.querySelector("#open-tasks"),
  openSkills: document.querySelector("#open-skills"),
  settingsButton: document.querySelector("#settings-button"),
  settingsDialog: document.querySelector("#settings-dialog"),
  themeChoices: document.querySelectorAll("[data-theme-choice]"),
  chatFontChoices: document.querySelectorAll("[data-chat-font-size]"),
  newChannel: document.querySelector("#new-channel"),
  manageChannels: document.querySelector("#manage-channels"),
  newAgent: document.querySelector("#new-agent"),
  manageAgents: document.querySelector("#manage-agents"),
  roomEyebrow: document.querySelector("#room-eyebrow"),
  roomName: document.querySelector("#room-name"),
  defaultAgentControl: document.querySelector(".default-agent-control"),
  daemonChip: document.querySelector("#daemon-chip"),
  debugToggle: document.querySelector("#debug-toggle"),
  debugClose: document.querySelector("#debug-close"),
  inspector: document.querySelector("#debug-inspector"),
  daemonCount: document.querySelector("#daemon-count"),
  taskContext: document.querySelector("#task-context"),
  messages: document.querySelector("#messages"),
  skillManager: document.querySelector("#skill-manager"),
  skillManagerCount: document.querySelector("#skill-manager-count"),
  skillManagerAdd: document.querySelector("#skill-manager-add"),
  skillSearch: document.querySelector("#skill-search"),
  skillTagFilter: document.querySelector("#skill-tag-filter"),
  skillManagerList: document.querySelector("#skill-manager-list"),
  taskManager: document.querySelector("#task-manager"),
  taskManagerCount: document.querySelector("#task-manager-count"),
  taskManagerAdd: document.querySelector("#task-manager-add"),
  taskLaneAdd: document.querySelector("#task-lane-add"),
  taskBoard: document.querySelector("#task-board"),
  taskChatDrawer: document.querySelector("#task-chat-drawer"),
  taskChatTitle: document.querySelector("#task-chat-title"),
  taskChatMeta: document.querySelector("#task-chat-meta"),
  taskChatOpenChannel: document.querySelector("#task-chat-open-channel"),
  taskChatClose: document.querySelector("#task-chat-close"),
  taskChatMessages: document.querySelector("#task-chat-messages"),
  taskChatComposer: document.querySelector("#task-chat-composer"),
  taskChatMentions: document.querySelector("#task-chat-mentions"),
  taskChatInput: document.querySelector("#task-chat-input"),
  composer: document.querySelector("#composer"),
  input: document.querySelector("#message-input"),
  mentionRow: document.querySelector("#mention-row"),
  mentionSuggestions: document.querySelector("#mention-suggestions"),
  assignAgent: document.querySelector("#assign-agent"),
  assignTask: document.querySelector("#assign-task"),
  assignButton: document.querySelector("#assign-button"),
  defaultAgent: document.querySelector("#default-agent"),
  eventList: document.querySelector("#event-list"),
  eventDetail: document.querySelector("#event-detail"),
  eventCount: document.querySelector("#event-count"),
  agentDialog: document.querySelector("#agent-dialog"),
  agentDialogTitle: document.querySelector("#agent-dialog-title"),
  agentName: document.querySelector("#agent-name"),
  agentPersona: document.querySelector("#agent-persona"),
  agentSystemPrompt: document.querySelector("#agent-system-prompt"),
  agentRuntime: document.querySelector("#agent-runtime"),
  agentModel: document.querySelector("#agent-model"),
  agentModelCustom: document.querySelector("#agent-model-custom"),
  agentModelCustomRow: document.querySelector("#agent-model-custom-row"),
  agentSkills: document.querySelector("#agent-skills"),
  agentSkillsRow: document.querySelector("#agent-skills-row"),
  agentSkillPicker: document.querySelector("#agent-skill-picker"),
  agentSkillLibrary: document.querySelector("#agent-skill-library"),
  agentSubmit: document.querySelector("#agent-create"),
  taskDialog: document.querySelector("#task-dialog"),
  taskDialogTitle: document.querySelector("#task-dialog-title"),
  taskSubmit: document.querySelector("#task-create"),
  taskTitle: document.querySelector("#task-title"),
  taskDescription: document.querySelector("#task-description"),
  taskWorkdir: document.querySelector("#task-workdir"),
  taskLane: document.querySelector("#task-lane"),
  taskLaneDialog: document.querySelector("#task-lane-dialog"),
  taskLaneList: document.querySelector("#task-lane-list"),
  taskLaneName: document.querySelector("#task-lane-name"),
  userDialog: document.querySelector("#user-dialog"),
  userName: document.querySelector("#user-name"),
  skillDialog: document.querySelector("#skill-dialog"),
  skillDialogTitle: document.querySelector("#skill-dialog-title"),
  skillAgentName: document.querySelector("#skill-agent-name"),
  skillList: document.querySelector("#skill-list"),
  skillAttachRow: document.querySelector("#skill-attach-row"),
  skillAttachSelect: document.querySelector("#skill-attach-select"),
  skillModeSwitch: document.querySelector(".skill-mode-switch"),
  skillModeButtons: document.querySelectorAll("[data-skill-mode]"),
  skillCreateFields: document.querySelector("#skill-create-fields"),
  skillName: document.querySelector("#skill-name"),
  skillNameLabel: document.querySelector("#skill-name-label"),
  skillSource: document.querySelector("#skill-source"),
  skillSourceLabel: document.querySelector("#skill-source-label"),
  skillTags: document.querySelector("#skill-tags"),
  skillLocalFields: document.querySelector("#skill-local-fields"),
  skillFile: document.querySelector("#skill-file"),
  skillContent: document.querySelector("#skill-content"),
  skillImport: document.querySelector("#skill-import"),
  skillError: document.querySelector("#skill-error"),
  channelDialog: document.querySelector("#channel-dialog"),
  markdownDialog: document.querySelector("#markdown-dialog"),
  markdownDialogTitle: document.querySelector("#markdown-dialog-title"),
  markdownDialogMeta: document.querySelector("#markdown-dialog-meta"),
  markdownDialogBody: document.querySelector("#markdown-dialog-body"),
  markdownDialogClose: document.querySelector("#markdown-dialog-close"),
};

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || res.statusText);
  }
  return res.json();
}

async function load() {
  state.snapshot = await api("/api/state");
  render();
  const source = new EventSource("/api/events");
  source.addEventListener("snapshot", (event) => {
    state.snapshot = JSON.parse(event.data);
    render();
  });
}

function render() {
  if (!state.snapshot) return;
  const channels = state.snapshot.channels || [];
  const users = state.snapshot.users || [];
  const agents = state.snapshot.agents || [];
  const daemons = state.snapshot.daemons || [];
  const events = state.snapshot.events || [];
  const tasks = state.snapshot.tasks || [];
  const taskLanes = state.snapshot.taskLanes || [];
  const sidebarChannels = visibleSidebarChannels(channels, tasks, taskLanes);
  if (!channels.some((channel) => channel.id === state.channelId)) {
    state.channelId = channels[0]?.id || "";
  }
  const current = channels.find((channel) => channel.id === state.channelId);
  const currentTask = taskForChannel(current, tasks);
  const currentTaskLane = currentTask ? taskLanes.find((lane) => lane.id === currentTask.laneId) : null;
  const isSkillView = state.view === "skills";
  const isTaskView = state.view === "tasks";
  const isManagementView = isSkillView || isTaskView;
  els.roomEyebrow.textContent = isManagementView ? "Management" : currentTask ? "Task Channel" : "Channel";
  els.roomName.textContent = isTaskView ? "Tasks" : isSkillView ? "Skill Center" : current ? `#${current.name}` : "#channel";
  els.defaultAgentControl.hidden = isManagementView;
  renderTaskChannelContext(currentTask, currentTaskLane, users, agents, isManagementView);
  els.messages.hidden = isManagementView;
  els.composer.hidden = isManagementView;
  els.skillManager.hidden = !isSkillView;
  els.taskManager.hidden = !isTaskView;
  els.openTasks.classList.toggle("active", isTaskView);
  els.openSkills.classList.toggle("active", isSkillView);

  initializeChannelReadState(channels);
  if (!isManagementView && current) markChannelRead(current.id);
  if (isTaskView && state.taskDrawerChannelId) markChannelRead(state.taskDrawerChannelId);
  renderChannels(sidebarChannels);
  renderUsers(users);
  renderAgents(agents);
  if (isSkillView) {
    renderSkillManager(state.snapshot.skills || [], agents);
  } else if (isTaskView) {
    renderTaskManager(taskLanes, tasks, channels, users, agents);
  } else {
    closeTaskChatDrawer({ renderNow: false });
    renderMessages();
    renderMentions(availableMentionAgents(current, agents));
  }
  renderTaskChatDrawer(tasks, channels, users, agents, taskLanes, isTaskView);
  renderDaemon(daemons);
  renderDebugMode();
  renderChannelSettings(current, agents);
  renderAssign(agents);
  renderEvents(events);
  if (state.skillAgentId && els.skillDialog.open) renderSkillDialog();
}

function renderChannels(channels) {
  els.channelList.innerHTML = "";
  els.channelList.classList.toggle("manage-mode", state.channelManageMode);
  els.manageChannels.classList.toggle("active", state.channelManageMode);
  els.manageChannels.textContent = state.channelManageMode ? "Done" : "Manage";
  els.manageChannels.setAttribute("aria-pressed", state.channelManageMode ? "true" : "false");
  for (const channel of channels) {
    const row = document.createElement("div");
    row.className = `nav-row ${state.channelManageMode ? "" : "no-row-action"}`;
    const button = document.createElement("button");
    const isActive = state.view === "channel" && channel.id === state.channelId;
    const unread = isActive ? 0 : channelUnreadCount(channel.id);
    button.className = `nav-item ${isActive ? "active" : ""} ${unread ? "has-unread" : ""}`;
    button.dataset.tooltip = channel.topic ? `#${channel.name}\n${channel.topic}` : `#${channel.name}`;
    button.setAttribute("aria-label", unread ? `#${channel.name}, ${unread} unread` : `#${channel.name}`);
    button.innerHTML = `
      <span class="hash">#</span>
      <span><strong>${escapeHTML(channel.name)}</strong><span class="nav-topic">${escapeHTML(channel.topic || "")}</span></span>
      ${unread ? `<span class="unread-badge" aria-hidden="true">${escapeHTML(formatUnreadCount(unread))}</span>` : ""}`;
    button.addEventListener("click", () => {
      state.view = "channel";
      state.channelId = channel.id;
      markChannelRead(channel.id);
      render();
    });
    row.append(button);
    if (state.channelManageMode) {
      const deleteButton = document.createElement("button");
      deleteButton.type = "button";
      deleteButton.className = "item-delete visible";
      deleteButton.title = `Delete #${channel.name}`;
      deleteButton.setAttribute("aria-label", `Delete channel ${channel.name}`);
      deleteButton.textContent = "x";
      deleteButton.addEventListener("click", () => deleteChannel(channel));
      row.append(deleteButton);
    }
    els.channelList.append(row);
  }
}

function renderUsers(users) {
  els.userList.innerHTML = "";
  const currentUserId = state.snapshot.currentUserId || "usr_you";
  for (const user of users) {
    const row = document.createElement("div");
    row.className = "human-row";
    const item = document.createElement("div");
    item.className = "human-item";
    item.title = user.id === currentUserId ? `${user.name} - current human` : `${user.name} - registered human`;
    item.tabIndex = 0;
    item.dataset.profileKind = "human";
    item.dataset.profileId = user.id;
    item.dataset.profileName = user.name;
    item.innerHTML = `<span class="avatar" style="background:${user.color || "#2563eb"}">${initials(user.name)}</span><span><strong>${escapeHTML(user.name)}</strong><span class="human-meta">${user.id === currentUserId ? "current human" : "human participant"}</span></span>`;
    row.append(item);

    if (user.id !== currentUserId) {
      const deleteButton = document.createElement("button");
      deleteButton.type = "button";
      deleteButton.className = "item-delete";
      deleteButton.title = `Delete ${user.name}`;
      deleteButton.setAttribute("aria-label", `Delete human ${user.name}`);
      deleteButton.textContent = "x";
      deleteButton.addEventListener("click", () => deleteUser(user));
      row.append(deleteButton);
    }
    els.userList.append(row);
  }
}

function renderAgents(agents) {
  els.agentList.innerHTML = "";
  els.agentList.classList.toggle("manage-mode", state.agentManageMode);
  els.manageAgents.classList.toggle("active", state.agentManageMode);
  els.manageAgents.textContent = state.agentManageMode ? "Done" : "Manage";
  els.manageAgents.setAttribute("aria-pressed", state.agentManageMode ? "true" : "false");
  const nextAgentStatusById = {};
  for (const agent of agents) {
    const row = document.createElement("div");
    row.className = "agent-row";
    const button = document.createElement("button");
    button.className = "agent-item";
    const status = normalizeAgentStatus(agent.status);
    const previousStatus = state.agentStatusById[agent.id];
    nextAgentStatusById[agent.id] = status;
    row.dataset.agentStatus = status;
    if (previousStatus && previousStatus !== status) {
      button.classList.add("status-changed");
    }
    const skillCount = (agent.skills || []).length;
    const skillMeta = ` · ${skillCount} skill${skillCount === 1 ? "" : "s"}`;
    const promptMeta = agent.systemPrompt ? " · system prompt" : "";
    const meta = `${agent.status} · ${runtimeLabel(agent)}${skillMeta}${promptMeta} · ${agent.persona}`;
    button.title = `${agent.name} - ${meta}`;
    button.dataset.profileKind = "agent";
    button.dataset.profileId = agent.id;
    button.dataset.profileName = agent.name;
    button.innerHTML = `<span class="avatar" style="background:${agent.color || "#2563eb"}">${initials(agent.name)}</span><span class="agent-copy"><span class="agent-name-line"><strong>${escapeHTML(agent.name)}</strong>${agentStatusIndicator(status)}</span><span class="agent-meta">${escapeHTML(meta)}</span></span>`;
    button.addEventListener("click", () => {
      insertMention(agent.name);
    });
    const skillButton = document.createElement("button");
    skillButton.type = "button";
    skillButton.className = "agent-action-button";
    skillButton.dataset.tooltip = `Manage skills\n${agent.name}`;
    skillButton.setAttribute("aria-label", `Manage skills for ${agent.name}`);
    skillButton.textContent = "✦";
    skillButton.addEventListener("click", () => openSkillDialog(agent));
    const editButton = document.createElement("button");
    editButton.type = "button";
    editButton.className = "agent-action-button";
    editButton.dataset.tooltip = `Edit agent\n${agent.name}`;
    editButton.setAttribute("aria-label", `Edit agent ${agent.name}`);
    editButton.textContent = "✎";
    editButton.addEventListener("click", () => openAgentDialog(agent));
    const actionGroup = document.createElement("div");
    actionGroup.className = "agent-actions";
    actionGroup.append(editButton, skillButton);
    if (state.agentManageMode) {
      const deleteButton = document.createElement("button");
      deleteButton.type = "button";
      deleteButton.className = "agent-action-button danger";
      deleteButton.dataset.tooltip = `Layoff\n${agent.name}`;
      deleteButton.setAttribute("aria-label", `Layoff agent ${agent.name}`);
      deleteButton.textContent = "x";
      deleteButton.addEventListener("click", () => deleteAgent(agent));
      actionGroup.append(deleteButton);
    }
    row.append(button, actionGroup);
    els.agentList.append(row);
  }
  state.agentStatusById = nextAgentStatusById;
}

function normalizeAgentStatus(status = "") {
  const normalized = status.toLowerCase().trim();
  if (["thinking", "starting", "idle", "waiting", "offline"].includes(normalized)) {
    return normalized;
  }
  return normalized || "idle";
}

function agentStatusIndicator(status) {
  const label = status || "idle";
  return `<span class="agent-status-indicator" data-status="${escapeHTML(label)}" title="${escapeHTML(label)}" aria-label="${escapeHTML(label)}"></span>`;
}

function renderMessages() {
  renderMessagesFor(els.messages, state.channelId);
}

function renderMessagesFor(container, channelId) {
  const messages = state.snapshot.messages || [];
  const agents = state.snapshot.agents || [];
  const users = state.snapshot.users || [];
  const byAgent = new Map(agents.map((agent) => [agent.id, agent]));
  const byUser = new Map((users || []).map((user) => [user.id, user]));
  const visible = messages.filter((message) => message.channelId === channelId);
  container.innerHTML = "";
  for (const message of visible) {
    container.append(createMessageElement(message, byAgent, byUser));
  }
  container.scrollTop = container.scrollHeight;
}

function createMessageElement(message, byAgent, byUser) {
  const item = document.createElement("article");
  item.className = `message ${message.kind || ""} ${message.authorKind || ""}`;
  const agent = byAgent.get(message.authorId);
  const user = byUser.get(message.authorId);
  const color = agent?.color || user?.color || (message.authorKind === "human" ? "#2563eb" : "#64748b");
  const content = renderMessageContent(message);
  const profileAttrs = message.authorKind === "agent" || message.authorKind === "human"
    ? `data-profile-kind="${escapeHTML(message.authorKind)}" data-profile-id="${escapeHTML(message.authorId)}" data-profile-name="${escapeHTML(message.authorName)}" tabindex="0"`
    : "";
  item.innerHTML = `
    <span class="avatar profile-anchor" style="background:${color}" ${profileAttrs}>${initials(message.authorName)}</span>
    <div>
      <div class="message-header">
        <span class="message-name profile-anchor" ${profileAttrs}>${escapeHTML(message.authorName)}</span>
        <span class="message-kind">${escapeHTML(message.authorKind)}</span>
        <time class="message-time">${formatTime(message.timestamp)}</time>
      </div>
      ${content}
    </div>`;
  const cards = item.querySelectorAll("[data-markdown-document]");
  for (const card of cards) {
    card.addEventListener("click", () => {
      const document = markdownDocumentParts(message).document;
      openMarkdownDocument(message, document);
    });
  }
  return item;
}

function renderTaskChannelContext(task, lane, users, agents, isManagementView) {
  if (isManagementView || !task) {
    els.taskContext.hidden = true;
    els.taskContext.innerHTML = "";
    return;
  }
  const updated = task.updatedAt || task.createdAt;
  els.taskContext.hidden = false;
  els.taskContext.innerHTML = `
    <div class="task-context-icon" aria-hidden="true">Tk</div>
    <div class="task-context-main">
      <div class="task-context-meta">
        <span>${escapeHTML(lane?.name || "Unplanned")}</span>
        <span>${escapeHTML(taskAssigneeLabel(task, users, agents))}</span>
        <span>Updated ${escapeHTML(formatTaskTime(updated))}</span>
        ${task.workdir ? `<span title="${escapeHTML(task.workdir)}">cwd ${escapeHTML(compactPath(task.workdir))}</span>` : ""}
        <span>${task.channelId ? "Discussion channel" : "No channel"}</span>
      </div>
      <strong>${escapeHTML(task.title)}</strong>
      ${task.description ? `<p>${escapeHTML(task.description)}</p>` : ""}
    </div>
    <button type="button" class="item-action visible" data-action="open-task-board">Open Task</button>`;
  els.taskContext.querySelector("[data-action='open-task-board']").addEventListener("click", () => viewTaskOnBoard(task.id));
}

function renderTaskChatDrawer(tasks, channels, users, agents, lanes, isTaskView) {
  if (!isTaskView || !state.taskDrawerTaskId) {
    els.taskChatDrawer.hidden = true;
    return;
  }
  const task = tasks.find((candidate) => candidate.id === state.taskDrawerTaskId);
  if (!task) {
    closeTaskChatDrawer({ renderNow: false });
    return;
  }
  const channelID = state.taskDrawerChannelId || task.channelId;
  const channel = channels.find((candidate) => candidate.id === channelID);
  if (!channel) {
    els.taskChatDrawer.hidden = true;
    return;
  }
  state.taskDrawerChannelId = channel.id;
  const lane = lanes.find((candidate) => candidate.id === task.laneId);
  els.taskChatDrawer.hidden = false;
  els.taskChatTitle.textContent = task.title;
  els.taskChatMeta.textContent = `${lane?.name || "Unplanned"} · ${taskAssigneeLabel(task, users, agents)}${task.workdir ? ` · cwd ${compactPath(task.workdir)}` : ""} · #${channel.name}`;
  renderMessagesFor(els.taskChatMessages, channel.id);
  renderTaskChatMentions(availableMentionAgents(channel, agents));
  if (els.taskChatInput.value !== state.taskDrawerDraft) {
    els.taskChatInput.value = state.taskDrawerDraft;
  }
  markChannelRead(channel.id);
}

function renderTaskChatMentions(agents) {
  els.taskChatMentions.innerHTML = "";
  for (const agent of agents) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "mention-button";
    button.textContent = `@${agent.name}`;
    button.addEventListener("click", () => insertTaskChatMention(agent.name));
    els.taskChatMentions.append(button);
  }
}

function closeTaskChatDrawer(options = {}) {
  state.taskDrawerTaskId = "";
  state.taskDrawerChannelId = "";
  state.taskDrawerDraft = "";
  els.taskChatDrawer.hidden = true;
  if (options.renderNow !== false) render();
}

function renderMessageContent(message) {
  if (!isMarkdownDocumentMessage(message)) {
    return renderChatBlocks(message.text);
  }
  const parts = markdownDocumentParts(message);
  if (!shouldCollapseMarkdownDocument(parts.document)) {
    const before = parts.before ? renderChatBlocks(parts.before) : "";
    const document = renderInlineMarkdownDocument(parts.document);
    const after = parts.after ? renderChatBlocks(parts.after) : "";
    return `${before}${document}${after}`;
  }
  const title = markdownDocumentTitle(parts.document);
  const stats = markdownStats(parts.document);
  const excerpt = markdownExcerpt(parts.document, title);
  const before = parts.before ? renderChatBlocks(parts.before) : "";
  const after = parts.after ? renderChatBlocks(parts.after) : "";
  return `
    ${before}
    <button class="markdown-card" type="button" data-markdown-document="${escapeHTML(message.id || "")}">
      <span class="markdown-card-icon">MD</span>
      <span class="markdown-card-copy">
        <strong>${escapeHTML(title)}</strong>
        <small>${stats}</small>
        <span>${linkMentions(escapeHTML(excerpt))}</span>
      </span>
      <span class="markdown-card-action">Open</span>
    </button>
    ${after}`;
}

function renderChatBlocks(text = "") {
  const rendered = renderMarkdown(normalizeChatBlockMarkdown(text || ""));
  return rendered ? `<div class="message-blocks">${rendered}</div>` : "";
}

function renderInlineMarkdownDocument(text = "") {
  const rendered = renderMarkdown(text || "");
  return rendered ? `<div class="message-blocks inline-markdown-document">${rendered}</div>` : "";
}

function shouldCollapseMarkdownDocument(text = "") {
  const trimmed = text.trim();
  if (!trimmed) return false;
  const lines = trimmed.split("\n");
  const nonEmptyLines = lines.filter((line) => line.trim()).length;
  return trimmed.length >= markdownDocumentCardMinChars || nonEmptyLines >= markdownDocumentCardMinLines;
}

function isMarkdownDocumentMessage(message) {
  if (message.authorKind !== "agent") return false;
  const text = message.text || "";
  if (text.includes(markdownDocumentStartMarker)) return true;
  if (isPeerDiscussionMessage(message)) return false;
  if (text.length < 900) return false;
  const signals = [
    /^#{1,3}\s+/m,
    /\n```/,
    /\n\s*[-*]\s+\S/,
    /\n\s*\d+\.\s+\S/,
    /\n\|.+\|/,
    /\*\*[^*]+\*\*/,
  ];
  const signalCount = signals.filter((pattern) => pattern.test(text)).length;
  return signalCount >= 2 || text.length > 1600;
}

function isPeerDiscussionMessage(message) {
  if (message.authorKind !== "agent") return false;
  const tokens = mentionTokens(message.text || "");
  if (tokens.size === 0 || !state.snapshot) return false;
  const authorID = (message.authorId || "").toLowerCase();
  return (state.snapshot.agents || []).some((agent) => {
    if ((agent.id || "").toLowerCase() === authorID) return false;
    return tokens.has(agentMentionToken(agent.name)) || tokens.has(agentMentionToken(agent.id)) || tokens.has(agentShortIDToken(agent.id));
  });
}

function mentionTokens(text) {
  const tokens = new Set();
  for (const match of text.matchAll(mentionTokenRegex())) {
    const token = mentionKey(match[1]);
    if (token) tokens.add(token);
  }
  return tokens;
}

function agentMentionToken(value = "") {
  return mentionKey(value);
}

function agentShortIDToken(value = "") {
  return agentMentionToken(value).replace(/^agent_/, "");
}

function markdownDocumentParts(message) {
  const text = message.text || "";
  const marked = markedMarkdownDocumentParts(text);
  if (marked) return marked;
  if (!isMarkdownDocumentMessage(message)) {
    return { before: text, document: "", after: "" };
  }
  const lines = text.split("\n");
  const start = markdownDocumentStartLine(lines);
  if (start <= 0) {
    return { before: "", document: text.trim(), after: "" };
  }
  const legacy = legacyMarkdownDocumentParts(lines, start);
  if (legacy.document) return legacy;
  return {
    before: lines.slice(0, start).join("\n").trim(),
    document: lines.slice(start).join("\n").trim(),
    after: "",
  };
}

function markedMarkdownDocumentParts(text) {
  const start = text.indexOf(markdownDocumentStartMarker);
  if (start === -1) return null;
  const bodyStart = start + markdownDocumentStartMarker.length;
  const end = text.indexOf(markdownDocumentEndMarker, bodyStart);
  if (end === -1) {
    return {
      before: text.slice(0, start).trim(),
      document: text.slice(bodyStart).trim(),
      after: "",
    };
  }
  return {
    before: text.slice(0, start).trim(),
    document: text.slice(bodyStart, end).trim(),
    after: text.slice(end + markdownDocumentEndMarker.length).trim(),
  };
}

function legacyMarkdownDocumentParts(lines, start) {
  let documentLines = lines.slice(start);
  let afterLines = [];
  const handoff = trailingHandoffLine(documentLines);
  if (handoff !== -1) {
    let split = handoff;
    const previous = previousNonEmptyLine(documentLines, handoff - 1);
    if (previous !== -1 && isConversationalTailLine(documentLines[previous])) {
      split = previous;
    }
    afterLines = documentLines.slice(split);
    documentLines = documentLines.slice(0, split);
  }
  return {
    before: lines.slice(0, start).join("\n").trim(),
    document: documentLines.join("\n").trim(),
    after: afterLines.join("\n").trim(),
  };
}

function trailingHandoffLine(lines) {
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const trimmed = lines[index].trim();
    if (!trimmed) continue;
    return /^@You\b/.test(trimmed) ? index : -1;
  }
  return -1;
}

function previousNonEmptyLine(lines, start) {
  for (let index = start; index >= 0; index -= 1) {
    if (lines[index].trim()) return index;
  }
  return -1;
}

function isConversationalTailLine(line) {
  const trimmed = line.trim();
  if (!trimmed || isMarkdownStructuralLine(trimmed)) return false;
  if (/^@[^\s@<>()\[\]{}.,，。:：;；!?！？]+(?:\s|$)/.test(trimmed)) return true;
  if (/^(唯一分歧|最后|结论|补充|备注|注意)[:：]/.test(trimmed)) return true;
  return false;
}

function isMarkdownStructuralLine(trimmed) {
  return (
    /^#{1,6}\s+/.test(trimmed) ||
    /^[-*]\s+/.test(trimmed) ||
    /^\d+\.\s+/.test(trimmed) ||
    /^```/.test(trimmed) ||
    /^\|.+\|$/.test(trimmed) ||
    /^\*\*[^*]+\*\*$/.test(trimmed) ||
    trimmed === "---"
  );
}

function markdownDocumentStartLine(lines) {
  for (let index = 0; index < lines.length; index += 1) {
    const trimmed = lines[index].trim();
    if (!trimmed) continue;
    if (trimmed === "---" && followingMarkdownSignals(lines, index + 1) >= 1) return index;
    if (/^#{1,3}\s+/.test(trimmed)) return index;
    if (/^\*\*[^*]{2,120}\*\*$/.test(trimmed) && followingMarkdownSignals(lines, index + 1) >= 1) return index;
  }
  return 0;
}

function followingMarkdownSignals(lines, start) {
  const sample = lines.slice(start, start + 8).join("\n");
  return [
    /^#{1,3}\s+/m,
    /\n\s*[-*]\s+\S/,
    /\n\s*\d+\.\s+\S/,
    /\n\|.+\|/,
    /\*\*[^*]+\*\*/,
    /```/,
  ].filter((pattern) => pattern.test(sample)).length;
}

function markdownDocumentTitle(text) {
  const heading = text.match(/^#{1,3}\s+(.+)$/m);
  if (heading?.[1]) return cleanMarkdownInline(heading[1]).slice(0, 90);
  const bold = text.match(/\*\*([^*]+)\*\*/);
  if (bold?.[1]) return cleanMarkdownInline(bold[1]).slice(0, 90);
  const firstLine = text
    .split("\n")
    .map((line) => cleanMarkdownInline(line))
    .find((line) => line && !line.startsWith("@"));
  return (firstLine || "Markdown document").slice(0, 90);
}

function markdownExcerpt(text, title) {
  const lines = text
    .split("\n")
    .map((line) => cleanMarkdownInline(line))
    .filter((line) => line && line !== title && !line.startsWith("---"));
  return (lines.find((line) => !line.startsWith("#")) || "Open to read the full document.").slice(0, 150);
}

function markdownStats(text) {
  const words = text.trim().split(/\s+/).filter(Boolean).length;
  const lines = text.split("\n").length;
  const minutes = Math.max(1, Math.ceil(words / 260));
  return `${lines} lines · ${minutes} min read`;
}

function cleanMarkdownInline(text = "") {
  return text
    .replace(/^#{1,6}\s+/, "")
    .replace(/^[-*]\s+/, "")
    .replace(/^\d+\.\s+/, "")
    .replace(/\*\*([^*]+)\*\*/g, "$1")
    .replace(/`([^`]+)`/g, "$1")
    .trim();
}

function openMarkdownDocument(message, documentText = message.text) {
  els.markdownDialogTitle.textContent = markdownDocumentTitle(documentText);
  els.markdownDialogMeta.textContent = `${message.authorName} · ${formatTime(message.timestamp)}`;
  els.markdownDialogBody.innerHTML = renderMarkdown(documentText);
  els.markdownDialogBody.scrollTop = 0;
  els.markdownDialog.showModal();
}

function renderMentions(agents) {
  els.mentionRow.innerHTML = "";
  for (const agent of agents) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "mention-button";
    button.textContent = `@${agent.name}`;
    button.addEventListener("click", () => insertMention(agent.name));
    els.mentionRow.append(button);
  }
}

function availableMentionAgents(channel, agents) {
  if (!channel) return agents;
  const members = new Set(channel.memberIds || []);
  const channelAgents = agents.filter((agent) => members.has(agent.id));
  return channelAgents.length ? channelAgents : agents;
}

function currentMentionAgents() {
  if (!state.snapshot) return [];
  const channel = state.snapshot.channels.find((candidate) => candidate.id === state.channelId);
  return availableMentionAgents(channel, state.snapshot.agents || []);
}

function updateMentionSuggestions() {
  const match = activeMentionToken();
  if (!match) {
    hideMentionSuggestions();
    return;
  }

  const query = decodeMentionToken(match.query).toLowerCase();
  if (!state.mention.active || state.mention.query !== match.query || state.mention.start !== match.start) {
    state.mention.selected = 0;
  }
  state.mention.active = true;
  state.mention.start = match.start;
  state.mention.query = match.query;

  const matches = currentMentionAgents()
    .filter((agent) => agent.name.toLowerCase().includes(query) || agent.id.toLowerCase().includes(query))
    .slice(0, 8);

  if (matches.length === 0) {
    hideMentionSuggestions();
    return;
  }

  if (state.mention.selected >= matches.length) state.mention.selected = 0;
  els.mentionSuggestions.hidden = false;
  els.mentionSuggestions.innerHTML = matches
    .map((agent, index) => mentionSuggestionHTML(agent, index === state.mention.selected))
    .join("");

  els.mentionSuggestions.querySelectorAll("[data-agent-id]").forEach((button, index) => {
    button.addEventListener("mousedown", (event) => {
      event.preventDefault();
      selectMention(matches[index]);
    });
  });
}

function activeMentionToken() {
  const cursor = els.input.selectionStart || 0;
  const text = els.input.value.slice(0, cursor);
  const start = text.lastIndexOf("@");
  if (start === -1) return null;
  const before = start === 0 ? "" : text[start - 1];
  if (before && /[a-z0-9_-]/i.test(before)) return null;
  const query = text.slice(start + 1);
  if (/\s/.test(query) || query.includes("@")) return null;
  return { start, end: cursor, query };
}

function mentionSuggestionHTML(agent, selected) {
  return `
    <button type="button" class="mention-suggestion ${selected ? "active" : ""}" data-agent-id="${escapeHTML(agent.id)}">
      <span class="avatar" style="background:${agent.color || "#2563eb"}">${initials(agent.name)}</span>
      <span>
        <strong>@${escapeHTML(agent.name)}</strong>
        <small>${escapeHTML(runtimeLabel(agent))}</small>
      </span>
    </button>`;
}

function hideMentionSuggestions() {
  state.mention.active = false;
  state.mention.start = -1;
  state.mention.query = "";
  state.mention.selected = 0;
  els.mentionSuggestions.hidden = true;
  els.mentionSuggestions.innerHTML = "";
}

function selectMention(agent) {
  const match = activeMentionToken();
  if (!match) return;
  const before = els.input.value.slice(0, match.start);
  const after = els.input.value.slice(match.end);
  const mention = `${displayMentionHandle(agent.name)} `;
  const value = `${before}${mention}${after}`;
  const cursor = before.length + mention.length;
  els.input.value = value;
  els.input.setSelectionRange(cursor, cursor);
  hideMentionSuggestions();
  els.input.focus();
}

function currentMentionMatches() {
  const match = activeMentionToken();
  if (!match) return [];
  const query = match.query.toLowerCase();
  return currentMentionAgents()
    .filter((agent) => agent.name.toLowerCase().includes(query) || agent.id.toLowerCase().includes(query))
    .slice(0, 8);
}

function selectedMentionAgent() {
  const matches = currentMentionMatches();
  return matches[state.mention.selected] || matches[0] || null;
}

function moveMentionSelection(delta) {
  const matches = currentMentionMatches();
  if (matches.length === 0) {
    hideMentionSuggestions();
    return;
  }
  state.mention.selected = (state.mention.selected + delta + matches.length) % matches.length;
  updateMentionSuggestions();
}

function renderDaemon(daemons) {
  const online = daemons.filter((daemon) => daemon.status === "online");
  els.daemonChip.textContent = online.length ? `${online.length} daemon online` : "daemon offline";
  els.daemonChip.classList.toggle("online", online.length > 0);
  els.daemonCount.textContent = `${online.length} online`;
}

function renderDebugMode() {
  els.appShell.classList.toggle("debug-open", state.debugOpen);
  els.inspector.hidden = !state.debugOpen;
  els.debugToggle.classList.toggle("active", state.debugOpen);
  els.debugToggle.setAttribute("aria-expanded", state.debugOpen ? "true" : "false");
}

function renderChannelSettings(channel, agents) {
  const previous = els.defaultAgent.value;
  els.defaultAgent.innerHTML = "";
  for (const agent of agents) {
    const option = document.createElement("option");
    option.value = agent.id;
    option.textContent = `${agent.name} · ${runtimeLabel(agent)}`;
    els.defaultAgent.append(option);
  }
  const selected = channelDefaultAgentId(channel, agents) || previous;
  if (selected) els.defaultAgent.value = selected;
  const disabled = !channel || agents.length === 0;
  els.defaultAgent.disabled = disabled;
}

function renderAssign(agents) {
  const previous = els.assignAgent.value;
  els.assignAgent.innerHTML = "";
  for (const agent of agents) {
    const option = document.createElement("option");
    option.value = agent.id;
    option.textContent = `${agent.name} (${agent.status})`;
    els.assignAgent.append(option);
  }
  if (previous) els.assignAgent.value = previous;
}

function renderEvents(events) {
  els.eventCount.textContent = String(events.length);
  const latest = [...events].slice(-24).reverse();
  if (!state.selectedEventId && latest[0]) state.selectedEventId = latest[0].id;
  els.eventList.innerHTML = "";
  for (const event of latest) {
    const row = document.createElement("button");
    row.className = `event-row ${event.id === state.selectedEventId ? "active" : ""}`;
    row.innerHTML = `<code>${escapeHTML(event.type)}</code><span>${formatTime(event.ts)}</span>`;
    row.addEventListener("click", () => {
      state.selectedEventId = event.id;
      renderEvents(events);
    });
    els.eventList.append(row);
  }
  const selected = events.find((event) => event.id === state.selectedEventId) || latest[0];
  els.eventDetail.textContent = selected ? JSON.stringify(selected, null, 2) : "{}";
}

function initialTheme() {
  try {
    return localStorage.getItem(themeStorageKey) === "dark" ? "dark" : "light";
  } catch {
    return "light";
  }
}

function applyTheme(theme) {
  const normalized = theme === "dark" ? "dark" : "light";
  state.theme = normalized;
  document.documentElement.dataset.theme = normalized;
  try {
    localStorage.setItem(themeStorageKey, normalized);
  } catch {
    // Theme persistence is optional.
  }
  updateThemeControls();
}

function updateThemeControls() {
  for (const choice of els.themeChoices) {
    const active = choice.dataset.themeChoice === state.theme;
    choice.classList.toggle("active", active);
    choice.setAttribute("aria-pressed", String(active));
  }
}

function initialChatFontSize() {
  try {
    const value = localStorage.getItem(chatFontSizeStorageKey) || "regular";
    return normalizeChatFontSize(value);
  } catch {
    return "regular";
  }
}

function normalizeChatFontSize(value) {
  return ["compact", "regular", "large", "xl"].includes(value) ? value : "regular";
}

function applyChatFontSize(size) {
  const normalized = normalizeChatFontSize(size);
  state.chatFontSize = normalized;
  document.documentElement.dataset.chatFontSize = normalized;
  try {
    localStorage.setItem(chatFontSizeStorageKey, normalized);
  } catch {
    // Chat font persistence is optional.
  }
  updateChatFontSizeControls();
}

function updateChatFontSizeControls() {
  for (const choice of els.chatFontChoices) {
    const active = choice.dataset.chatFontSize === state.chatFontSize;
    choice.classList.toggle("active", active);
    choice.setAttribute("aria-pressed", String(active));
  }
}

function initialChannelReadState() {
  try {
    const raw = localStorage.getItem(channelReadStorageKey);
    if (!raw) return { hydrated: false, channels: {} };
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return { hydrated: false, channels: {} };
    return {
      hydrated: true,
      channels: parsed.channels && typeof parsed.channels === "object" ? parsed.channels : {},
    };
  } catch {
    return { hydrated: false, channels: {} };
  }
}

function initializeChannelReadState(channels) {
  if (state.read.hydrated) return;
  for (const channel of channels) {
    markChannelRead(channel.id, { persist: false });
  }
  state.read.hydrated = true;
  persistChannelReadState();
}

function markChannelRead(channelId, options = {}) {
  if (!channelId || !state.snapshot) return;
  const latest = latestChannelMessage(channelId);
  const next = latest
    ? { messageId: latest.id || "", timestamp: latest.timestamp || "" }
    : { messageId: "", timestamp: "" };
  const previous = state.read.channels[channelId] || {};
  if (previous.messageId === next.messageId && previous.timestamp === next.timestamp) return;
  state.read.channels[channelId] = next;
  if (options.persist !== false) persistChannelReadState();
}

function forgetChannelRead(channelId) {
  if (!channelId) return;
  delete state.read.channels[channelId];
  persistChannelReadState();
}

function persistChannelReadState() {
  try {
    localStorage.setItem(channelReadStorageKey, JSON.stringify({ channels: state.read.channels }));
  } catch {
    // Unread badges still work for this session if storage is unavailable.
  }
}

function latestChannelMessage(channelId) {
  const messages = state.snapshot?.messages || [];
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].channelId === channelId) return messages[index];
  }
  return null;
}

function channelUnreadCount(channelId) {
  const messages = (state.snapshot?.messages || []).filter((message) => message.channelId === channelId);
  if (messages.length === 0) return 0;
  const cursor = state.read.channels[channelId];
  if (!cursor) return messages.length;

  const cursorIndex = messages.findIndex((message) => message.id === cursor.messageId);
  if (cursorIndex !== -1) return Math.max(0, messages.length - cursorIndex - 1);

  const cursorTime = messageTimeValue(cursor);
  return messages.filter((message) => messageTimeValue(message) > cursorTime).length;
}

function messageTimeValue(message) {
  const parsed = Date.parse(message.timestamp || "");
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatUnreadCount(count) {
  return count > 99 ? "99+" : String(count);
}

async function sendComposerMessage() {
  const text = encodeMessageMentionsForWire(els.input.value.trim());
  if (!text) return;
  els.input.value = "";
  hideMentionSuggestions();
  try {
    await api("/api/messages", {
      method: "POST",
      body: JSON.stringify({ channelId: state.channelId, text }),
    });
  } catch (error) {
    alert(error.message);
  }
}

async function sendTaskChatMessage() {
  const text = encodeMessageMentionsForWire(els.taskChatInput.value.trim());
  const channelId = state.taskDrawerChannelId;
  if (!text || !channelId) return;
  els.taskChatInput.value = "";
  state.taskDrawerDraft = "";
  try {
    await api("/api/messages", {
      method: "POST",
      body: JSON.stringify({ channelId, text }),
    });
    state.snapshot = await api("/api/state");
    markChannelRead(channelId);
    render();
  } catch (error) {
    alert(error.message);
  }
}

els.composer.addEventListener("submit", async (event) => {
  event.preventDefault();
  await sendComposerMessage();
});

els.taskChatClose.addEventListener("click", () => closeTaskChatDrawer());
els.taskChatOpenChannel.addEventListener("click", () => {
  if (!state.taskDrawerChannelId) return;
  state.view = "channel";
  state.channelId = state.taskDrawerChannelId;
  closeTaskChatDrawer({ renderNow: false });
  markChannelRead(state.channelId);
  render();
});
els.taskChatComposer.addEventListener("submit", async (event) => {
  event.preventDefault();
  await sendTaskChatMessage();
});
els.taskChatInput.addEventListener("input", () => {
  state.taskDrawerDraft = els.taskChatInput.value;
});
els.taskChatInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.isComposing && (event.metaKey || event.ctrlKey)) {
    event.preventDefault();
    insertTaskChatTextAtCursor("\n");
    return;
  }
  if (event.key === "Enter" && !event.isComposing) {
    event.preventDefault();
    void sendTaskChatMessage();
  }
});

els.input.addEventListener("input", updateMentionSuggestions);
els.input.addEventListener("click", updateMentionSuggestions);
els.input.addEventListener("keyup", (event) => {
  if (["ArrowUp", "ArrowDown", "Enter", "Tab", "Escape"].includes(event.key)) return;
  updateMentionSuggestions();
});
els.input.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.isComposing && (event.metaKey || event.ctrlKey)) {
    event.preventDefault();
    insertTextAtCursor("\n");
    hideMentionSuggestions();
    return;
  }

  if (els.mentionSuggestions.hidden) {
    if (event.key === "Enter" && !event.isComposing) {
      event.preventDefault();
      void sendComposerMessage();
    }
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    hideMentionSuggestions();
    return;
  }
  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    event.preventDefault();
    moveMentionSelection(event.key === "ArrowDown" ? 1 : -1);
    return;
  }
  if (event.key === "Enter" || event.key === "Tab") {
    const selected = selectedMentionAgent();
    if (!selected) return;
    event.preventDefault();
    selectMention(selected);
  }
});
els.input.addEventListener("blur", () => {
  window.setTimeout(hideMentionSuggestions, 120);
});

document.addEventListener("mouseover", (event) => {
  const profileTarget = event.target.closest?.("[data-profile-kind][data-profile-id]");
  if (profileTarget) {
    hideHoverTooltip();
    showProfilePopover(profileTarget);
    return;
  }
  const target = event.target.closest?.("[data-tooltip]");
  if (target) showHoverTooltip(target);
});

document.addEventListener("mousemove", () => {
  if (profilePopoverTarget) positionFloatingCard(profilePopover, profilePopoverTarget, 12);
  if (hoverTooltipTarget) positionHoverTooltip(hoverTooltipTarget);
});

document.addEventListener("mouseout", (event) => {
  if (profilePopoverTarget) {
    const related = event.relatedTarget;
    if (related instanceof Node && profilePopoverTarget.contains(related)) return;
    hideProfilePopover();
  }
  if (!hoverTooltipTarget) return;
  const related = event.relatedTarget;
  if (related instanceof Node && hoverTooltipTarget.contains(related)) return;
  hideHoverTooltip();
});

document.addEventListener("focusin", (event) => {
  const profileTarget = event.target.closest?.("[data-profile-kind][data-profile-id]");
  if (profileTarget) {
    hideHoverTooltip();
    showProfilePopover(profileTarget);
    return;
  }
  const target = event.target.closest?.("[data-tooltip]");
  if (target) showHoverTooltip(target);
});

document.addEventListener("focusout", () => {
  hideProfilePopover();
  hideHoverTooltip();
});
window.addEventListener("resize", () => {
  hideProfilePopover();
  hideHoverTooltip();
});
window.addEventListener("scroll", () => {
  hideProfilePopover();
  hideHoverTooltip();
}, true);

els.assignButton.addEventListener("click", async () => {
  const task = els.assignTask.value.trim();
  const agentId = els.assignAgent.value;
  if (!task || !agentId) return;
  els.assignTask.value = "";
  try {
    await api(`/api/agents/${encodeURIComponent(agentId)}/assign`, {
      method: "POST",
      body: JSON.stringify({ channelId: state.channelId, task }),
    });
  } catch (error) {
    alert(error.message);
  }
});

els.defaultAgent.addEventListener("change", async () => {
  const agentId = els.defaultAgent.value;
  if (!state.channelId || !agentId) return;
  try {
    await api(`/api/channels/${encodeURIComponent(state.channelId)}/default-agent`, {
      method: "POST",
      body: JSON.stringify({ agentId }),
    });
  } catch (error) {
    alert(error.message);
  }
});

els.newAgent.addEventListener("click", () => openAgentDialog());
els.manageAgents.addEventListener("click", () => {
  state.agentManageMode = !state.agentManageMode;
  render();
});
document.querySelector("#new-user").addEventListener("click", () => els.userDialog.showModal());
els.newChannel.addEventListener("click", () => els.channelDialog.showModal());
els.manageChannels.addEventListener("click", () => {
  state.channelManageMode = !state.channelManageMode;
  render();
});
els.openTasks.addEventListener("click", () => {
  state.view = "tasks";
  render();
});
els.openSkills.addEventListener("click", () => {
  state.view = "skills";
  render();
});
els.settingsButton.addEventListener("click", () => {
  els.settingsDialog.showModal();
});
for (const choice of els.themeChoices) {
  choice.addEventListener("click", () => {
    applyTheme(choice.dataset.themeChoice);
  });
}
for (const choice of els.chatFontChoices) {
  choice.addEventListener("click", () => {
    applyChatFontSize(choice.dataset.chatFontSize);
  });
}
els.debugToggle.addEventListener("click", () => {
  state.debugOpen = !state.debugOpen;
  renderDebugMode();
});
els.debugClose.addEventListener("click", () => {
  state.debugOpen = false;
  renderDebugMode();
  els.debugToggle.focus();
});
els.settingsDialog.addEventListener("close", () => {
  els.settingsButton.focus();
});
els.taskManagerAdd.addEventListener("click", () => openTaskDialog());
els.taskLaneAdd.addEventListener("click", openTaskLaneManager);
els.skillManagerAdd.addEventListener("click", () => {
  openGlobalSkillDialog();
});
els.skillSearch.addEventListener("input", () => {
  state.skillSearch = els.skillSearch.value.trim();
  renderSkillManager(state.snapshot?.skills || [], state.snapshot?.agents || []);
});
els.skillTagFilter.addEventListener("change", () => {
  state.skillTagFilter = els.skillTagFilter.value;
  renderSkillManager(state.snapshot?.skills || [], state.snapshot?.agents || []);
});

els.agentRuntime.addEventListener("change", () => populateModelOptions(els.agentRuntime.value));
els.agentModel.addEventListener("change", updateCustomModelVisibility);
els.agentName.addEventListener("input", sanitizeAgentNameField);
els.agentDialog.addEventListener("close", resetAgentDialog);
els.markdownDialogClose.addEventListener("click", () => els.markdownDialog.close());

function openAgentDialog(agent = null) {
  const editing = Boolean(agent);
  state.editingAgentId = editing ? agent.id : "";
  els.agentDialogTitle.textContent = editing ? "Edit Agent" : "Create Agent";
  els.agentSubmit.textContent = editing ? "Save" : "Create";
  els.agentName.value = agent?.name || "";
  els.agentPersona.value = agent?.persona || "";
  els.agentSystemPrompt.value = agent?.systemPrompt || "";
  els.agentRuntime.value = agent?.runtime || "codex";
  setAgentModelValue(els.agentRuntime.value, agent?.model || "");
  els.agentSkills.value = "";
  els.agentSkillsRow.hidden = editing;
  els.agentSkillPicker.hidden = editing;
  if (!editing) renderAgentSkillLibrary();
  els.agentDialog.showModal();
  els.agentName.focus();
}

document.querySelector("#agent-create").addEventListener("click", async (event) => {
  event.preventDefault();
  sanitizeAgentNameField();
  const name = normalizeAgentNameInput(els.agentName.value);
  const persona = els.agentPersona.value.trim();
  const systemPrompt = els.agentSystemPrompt.value.trim();
  const runtime = els.agentRuntime.value;
  const selectedModel = els.agentModel.value;
  const model = selectedModel === "__custom" ? els.agentModelCustom.value.trim() : selectedModel;
  const skills = parseInitialSkills(els.agentSkills.value);
  const skillIds = selectedAgentSkillIDs();
  if (!name) return;
  try {
    if (state.editingAgentId) {
      await api(`/api/agents/${encodeURIComponent(state.editingAgentId)}`, {
        method: "PATCH",
        body: JSON.stringify({ name, persona, systemPrompt, runtime, model }),
      });
    } else {
      await api("/api/agents", { method: "POST", body: JSON.stringify({ name, persona, systemPrompt, runtime, model, skills, skillIds }) });
    }
    resetAgentDialog();
    els.agentDialog.close();
  } catch (error) {
    alert(error.message);
  }
});

document.querySelector("#user-create").addEventListener("click", async (event) => {
  event.preventDefault();
  const name = els.userName.value.trim();
  if (!name) return;
  await api("/api/users", { method: "POST", body: JSON.stringify({ name }) });
  els.userName.value = "";
  els.userDialog.close();
});

els.skillFile.addEventListener("change", async () => {
  const file = els.skillFile.files?.[0];
  if (!file) return;
  clearSkillError();
  try {
    els.skillSource.value = els.skillSource.value.trim() || file.name;
    els.skillName.value = els.skillName.value.trim() || file.name.replace(/\.[^.]+$/, "");
    els.skillContent.value = await file.text();
  } catch (error) {
    setSkillError(`Could not read skill file: ${error.message}`);
  }
});

els.skillAttachSelect.addEventListener("change", () => {
  if (state.skillCreateMode === "attach") updateSkillCreateModeUI();
});

for (const button of els.skillModeButtons) {
  button.addEventListener("click", () => {
    state.skillCreateMode = ["attach", "cloud"].includes(button.dataset.skillMode) ? button.dataset.skillMode : "local";
    clearSkillError();
    updateSkillCreateModeUI();
  });
}

els.skillImport.addEventListener("click", async (event) => {
  event.preventDefault();
  if (state.skillCreateMode === "attach") {
    await attachSelectedSkill();
    return;
  }
  const name = els.skillName.value.trim();
  const source = els.skillSource.value.trim();
  const isCloudImport = state.skillCreateMode === "cloud";
  const content = isCloudImport ? "" : els.skillContent.value.trim();
  const tags = parseSkillTags(els.skillTags.value);
  clearSkillError();
  if (!name && !isCloudImport) {
    setSkillError("Skill name is required.");
    els.skillName.focus();
    return;
  }
  if (isCloudImport && !isSupportedCloudSkillURL(source)) {
    setSkillError("Cloud import supports skills.sh links, GitHub links, and npx commands containing one of those links.");
    els.skillSource.focus();
    return;
  }
  if (!isCloudImport && !content) {
    setSkillError("Add skill content by choosing a .md/.txt file or pasting instructions.");
    els.skillContent.focus();
    return;
  }
  try {
    if (state.skillDialogMode === "global") {
      await api("/api/skills", { method: "POST", body: JSON.stringify({ name, source, content, tags }) });
    } else {
      const agentId = state.skillAgentId;
      if (!agentId) {
        setSkillError("Choose an agent before importing a skill.");
        return;
      }
      await api(`/api/agents/${encodeURIComponent(agentId)}/skills`, {
        method: "POST",
        body: JSON.stringify({ name, source, content, tags }),
      });
    }
    state.snapshot = await api("/api/state");
    render();
    els.skillName.value = "";
    els.skillSource.value = "";
    els.skillTags.value = "";
    els.skillFile.value = "";
    els.skillContent.value = "";
    clearSkillError();
    renderSkillDialog();
  } catch (error) {
    setSkillError(error.message);
  }
});

async function attachSelectedSkill() {
  const agentId = state.skillAgentId;
  const skillId = els.skillAttachSelect.value;
  clearSkillError();
  if (!agentId) {
    setSkillError("Choose an agent before attaching a skill.");
    return;
  }
  if (!skillId) {
    setSkillError("No unattached Skill Center skill is selected.");
    return;
  }
  try {
    await api(`/api/agents/${encodeURIComponent(agentId)}/skills`, {
      method: "POST",
      body: JSON.stringify({ skillId }),
    });
    state.snapshot = await api("/api/state");
    render();
    renderSkillDialog();
  } catch (error) {
    setSkillError(error.message);
  }
}

document.querySelector("#channel-create").addEventListener("click", async (event) => {
  event.preventDefault();
  const name = document.querySelector("#channel-name").value.trim();
  const topic = document.querySelector("#channel-topic").value.trim();
  if (!name) return;
  const channel = await api("/api/channels", { method: "POST", body: JSON.stringify({ name, topic }) });
  state.channelId = channel.id;
  document.querySelector("#channel-name").value = "";
  document.querySelector("#channel-topic").value = "";
  els.channelDialog.close();
});

document.querySelector("#task-create").addEventListener("click", async (event) => {
  event.preventDefault();
  const title = els.taskTitle.value.trim();
  const description = els.taskDescription.value.trim();
  const workdir = els.taskWorkdir.value.trim();
  const laneId = els.taskLane.value;
  if (!title) return;
  try {
    if (state.editingTaskId) {
      await api(`/api/tasks/${encodeURIComponent(state.editingTaskId)}`, {
        method: "PATCH",
        body: JSON.stringify({ title, description, workdir, laneId }),
      });
    } else {
      await api("/api/tasks", { method: "POST", body: JSON.stringify({ title, description, workdir, laneId }) });
    }
    els.taskTitle.value = "";
    els.taskDescription.value = "";
    els.taskWorkdir.value = "";
    state.editingTaskId = "";
    state.snapshot = await api("/api/state");
    render();
    els.taskDialog.close();
  } catch (error) {
    alert(error.message);
  }
});

els.taskDialog.addEventListener("close", () => {
  state.editingTaskId = "";
  els.taskDialogTitle.textContent = "Create Task";
  els.taskSubmit.textContent = "Create";
});

document.querySelector("#task-lane-create").addEventListener("click", async (event) => {
  event.preventDefault();
  const name = els.taskLaneName.value.trim();
  if (!name) return;
  try {
    await api("/api/task-lanes", { method: "POST", body: JSON.stringify({ name }) });
    els.taskLaneName.value = "";
    state.snapshot = await api("/api/state");
    render();
    renderTaskLaneManager();
    els.taskLaneName.focus();
  } catch (error) {
    alert(error.message);
  }
});

function openTaskDialog(preferredLaneID = "", task = null) {
  const selectedLaneID = task?.laneId || preferredLaneID;
  state.editingTaskId = task?.id || "";
  renderTaskLanePicker(selectedLaneID);
  els.taskDialogTitle.textContent = task ? "Edit Task" : "Create Task";
  els.taskSubmit.textContent = task ? "Save Changes" : "Create";
  els.taskTitle.value = task?.title || "";
  els.taskDescription.value = task?.description || "";
  els.taskWorkdir.value = task?.workdir || "";
  els.taskDialog.showModal();
  els.taskTitle.focus();
}

function openTaskLaneManager() {
  els.taskLaneName.value = "";
  renderTaskLaneManager();
  els.taskLaneDialog.showModal();
  els.taskLaneName.focus();
}

function renderTaskLaneManager() {
  const lanes = sortedTaskLanes(state.snapshot?.taskLanes || []);
  const tasks = state.snapshot?.tasks || [];
  els.taskLaneList.innerHTML = "";
  if (lanes.length === 0) {
    const empty = document.createElement("div");
    empty.className = "task-lane-manager-empty";
    empty.textContent = "No lanes yet.";
    els.taskLaneList.append(empty);
    return;
  }
  for (const [index, lane] of lanes.entries()) {
    const count = tasks.filter((task) => task.laneId === lane.id).length;
    const row = document.createElement("div");
    row.className = "task-lane-manager-row";
    row.innerHTML = `
      <div>
        <strong>${escapeHTML(lane.name)}</strong>
        <small>${count} task${count === 1 ? "" : "s"}</small>
      </div>
      <div class="task-lane-manager-controls">
        <button type="button" data-action="move-up" ${index === 0 ? "disabled" : ""}>Up</button>
        <button type="button" data-action="move-down" ${index === lanes.length - 1 ? "disabled" : ""}>Down</button>
        <button type="button" data-action="delete-lane" ${lanes.length <= 1 ? "disabled" : ""}>Delete</button>
      </div>`;
    row.querySelector("[data-action='move-up']").addEventListener("click", () => moveTaskLane(lane, index - 1));
    row.querySelector("[data-action='move-down']").addEventListener("click", () => moveTaskLane(lane, index + 1));
    row.querySelector("[data-action='delete-lane']").addEventListener("click", () => deleteTaskLane(lane));
    els.taskLaneList.append(row);
  }
}

function renderTaskLanePicker(selectedID = "") {
  const lanes = sortedTaskLanes(state.snapshot?.taskLanes || []);
  els.taskLane.innerHTML = "";
  for (const lane of lanes) {
    const option = document.createElement("option");
    option.value = lane.id;
    option.textContent = lane.name;
    els.taskLane.append(option);
  }
  if (selectedID) els.taskLane.value = selectedID;
}

async function deleteChannel(channel) {
  const task = taskForChannel(channel);
  const prompt = task
    ? `Hide #${channel.name}? This task channel and its messages stay available from the task.`
    : `Delete #${channel.name}? This removes the channel and its messages.`;
  if (!window.confirm(prompt)) return;
  try {
    const result = await api(`/api/channels/${encodeURIComponent(channel.id)}`, { method: "DELETE" });
    if (result.hidden && state.snapshot) {
      state.snapshot.channels = state.snapshot.channels.map((candidate) => (candidate.id === result.id ? result : candidate));
    } else {
      forgetChannelRead(channel.id);
    }
    if (state.channelId === channel.id) {
      const next = visibleSidebarChannels(state.snapshot.channels || [], state.snapshot.tasks || [], state.snapshot.taskLanes || []).find(
        (candidate) => candidate.id !== channel.id,
      );
      state.channelId = next?.id || "";
    }
  } catch (error) {
    alert(error.message);
  }
}

async function moveTaskToLane(taskId, laneId) {
  if (!taskId || !laneId) return;
  try {
    await api(`/api/tasks/${encodeURIComponent(taskId)}`, {
      method: "PATCH",
      body: JSON.stringify({ laneId }),
    });
    state.snapshot = await api("/api/state");
    render();
  } catch (error) {
    alert(error.message);
  }
}

async function updateTaskAssignee(taskId, value) {
  if (!taskId) return;
  const [assigneeKind = "", assigneeId = ""] = (value || "").split(":", 2);
  try {
    await api(`/api/tasks/${encodeURIComponent(taskId)}`, {
      method: "PATCH",
      body: JSON.stringify({ assigneeKind, assigneeId }),
    });
    state.snapshot = await api("/api/state");
    render();
  } catch (error) {
    alert(error.message);
  }
}

async function openTaskChannel(task) {
  try {
    const result = await ensureTaskChannel(task);
    if (result.channel?.id) {
      state.view = "channel";
      state.channelId = result.channel.id;
      closeTaskChatDrawer({ renderNow: false });
      markChannelRead(result.channel.id);
      render();
    }
  } catch (error) {
    alert(error.message);
  }
}

async function openTaskChatDrawer(task) {
  try {
    const result = await ensureTaskChannel(task);
    if (result.channel?.id) {
      state.taskDrawerTaskId = result.task?.id || task.id;
      state.taskDrawerChannelId = result.channel.id;
      state.taskDrawerDraft = "";
      markChannelRead(result.channel.id);
      render();
      els.taskChatInput.focus();
    }
  } catch (error) {
    alert(error.message);
  }
}

async function ensureTaskChannel(task) {
  const result = await api(`/api/tasks/${encodeURIComponent(task.id)}/channel`, { method: "POST" });
  state.snapshot = await api("/api/state");
  return result;
}

function viewTaskOnBoard(taskId) {
  state.view = "tasks";
  state.focusTaskId = taskId;
  render();
}

async function deleteTask(task) {
  if (!window.confirm(`Delete task "${task.title}"? The discussion channel will stay available if one exists.`)) return;
  try {
    await api(`/api/tasks/${encodeURIComponent(task.id)}`, { method: "DELETE" });
  } catch (error) {
    alert(error.message);
  }
}

async function deleteTaskLane(lane) {
  if (!window.confirm(`Delete lane "${lane.name}"? Tasks in this lane move to another lane.`)) return;
  try {
    await api(`/api/task-lanes/${encodeURIComponent(lane.id)}`, { method: "DELETE" });
    state.snapshot = await api("/api/state");
    render();
    if (els.taskLaneDialog.open) renderTaskLaneManager();
  } catch (error) {
    alert(error.message);
  }
}

async function moveTaskLane(lane, position) {
  try {
    await api(`/api/task-lanes/${encodeURIComponent(lane.id)}`, {
      method: "PATCH",
      body: JSON.stringify({ position }),
    });
    state.snapshot = await api("/api/state");
    render();
    if (els.taskLaneDialog.open) renderTaskLaneManager();
  } catch (error) {
    alert(error.message);
  }
}

async function deleteAgent(agent) {
  if (!window.confirm(`Delete ${agent.name}? Existing messages from this agent stay in the channel history.`)) return;
  try {
    await api(`/api/agents/${encodeURIComponent(agent.id)}`, { method: "DELETE" });
  } catch (error) {
    alert(error.message);
  }
}

function openSkillDialog(agent) {
  if (!agent) {
    alert("Create an agent before attaching skills.");
    return;
  }
  state.skillDialogMode = "agent";
  state.skillCreateMode = "attach";
  state.skillAgentId = agent.id;
  els.skillName.value = "";
  els.skillSource.value = "";
  els.skillTags.value = "";
  els.skillFile.value = "";
  els.skillContent.value = "";
  clearSkillError();
  renderSkillDialog();
  els.skillDialog.showModal();
}

function openGlobalSkillDialog() {
  state.skillDialogMode = "global";
  state.skillCreateMode = "local";
  state.skillAgentId = "";
  els.skillName.value = "";
  els.skillSource.value = "";
  els.skillTags.value = "";
  els.skillFile.value = "";
  els.skillContent.value = "";
  clearSkillError();
  renderSkillDialog();
  els.skillDialog.showModal();
}

function setSkillError(message) {
  els.skillError.textContent = message;
  els.skillError.hidden = false;
}

function clearSkillError() {
  els.skillError.textContent = "";
  els.skillError.hidden = true;
}

function updateSkillCreateModeUI() {
  if (state.skillDialogMode === "global" && state.skillCreateMode === "attach") {
    state.skillCreateMode = "local";
  }
  const isAttach = state.skillCreateMode === "attach";
  const isCloudImport = state.skillCreateMode === "cloud";
  els.skillModeSwitch.classList.toggle("two-option", state.skillDialogMode === "global");
  for (const button of els.skillModeButtons) {
    button.hidden = button.dataset.skillMode === "attach" && state.skillDialogMode === "global";
    const active = button.dataset.skillMode === state.skillCreateMode;
    button.classList.toggle("active", active);
    button.setAttribute("aria-pressed", String(active));
  }
  els.skillAttachRow.hidden = !isAttach || state.skillDialogMode === "global";
  els.skillCreateFields.hidden = isAttach;
  els.skillLocalFields.hidden = isAttach || isCloudImport;
  els.skillNameLabel.textContent = isCloudImport ? "Skill name (optional)" : "Skill name";
  els.skillSourceLabel.textContent = isCloudImport ? "Cloud URL or npx command" : "Source note";
  els.skillSource.placeholder = isCloudImport
    ? "https://skills.sh/... or npx ... https://github.com/owner/repo/blob/main/path/SKILL.md"
    : "SKILL.md or internal note";
  const action = state.skillDialogMode === "global"
    ? (isCloudImport ? "Import" : "Create")
    : (isAttach ? "Attach" : (isCloudImport ? "Import & Attach" : "Create & Attach"));
  els.skillImport.textContent = action;
  els.skillImport.disabled = isAttach && !els.skillAttachSelect.value;
}

function renderSkillManager(skills, agents) {
  if (els.skillSearch.value !== state.skillSearch) els.skillSearch.value = state.skillSearch;
  renderSkillTagFilter(skills);
  const query = state.skillSearch.toLowerCase();
  const visible = skills.filter((skill) => {
    if (state.skillTagFilter && !(skill.tags || []).includes(state.skillTagFilter)) return false;
    if (!query) return true;
    return [skill.name, skill.source, skill.content, ...(skill.tags || [])]
      .join("\n")
      .toLowerCase()
      .includes(query);
  });

  els.skillManagerCount.textContent = `${skills.length} skill${skills.length === 1 ? "" : "s"}`;
  els.skillManagerList.innerHTML = "";
  els.skillManagerAdd.disabled = false;
  if (skills.length === 0) {
    const empty = document.createElement("div");
    empty.className = "skill-manager-empty";
    empty.innerHTML = "<strong>No skills in the library yet.</strong><span>Create reusable skills here, then attach them from an agent.</span>";
    els.skillManagerList.append(empty);
    return;
  }
  if (visible.length === 0) {
    const empty = document.createElement("div");
    empty.className = "skill-manager-empty";
    empty.innerHTML = "<strong>No matching skills.</strong><span>Adjust the search query.</span>";
    els.skillManagerList.append(empty);
    return;
  }

  for (const skill of visible) {
    const usage = skillUsage(skill, agents);
    const row = document.createElement("article");
    row.className = "skill-manager-row";
    row.innerHTML = `
      <div class="skill-manager-agent skill-manager-library-icon">
        <span class="skill-badge">Sk</span>
        <span><strong>Library skill</strong><small>${escapeHTML(usage)}</small></span>
      </div>
      <button class="skill-manager-copy" type="button">
        <strong>${escapeHTML(skill.name)}</strong>
        <span>${escapeHTML(skill.source || "manual import")} · ${skill.content ? `${skill.content.length} chars` : "empty"}${skill.tags?.length ? ` · ${skill.tags.length} tags` : ""}</span>
        ${skillTagsHTML(skill)}
        <small>${escapeHTML(skillExcerpt(skill.content))}</small>
      </button>
      <div class="skill-manager-actions">
        <button class="item-action visible" type="button" data-action="open">Open</button>
        <button class="item-delete visible" type="button" data-action="delete" aria-label="Delete skill ${escapeHTML(skill.name)}">x</button>
      </div>`;
    row.querySelector("[data-action='open']").addEventListener("click", () => openSkillContent(skill, usage));
    row.querySelector(".skill-manager-copy").addEventListener("click", () => openSkillContent(skill, usage));
    row.querySelector("[data-action='delete']").addEventListener("click", () => deleteGlobalSkill(skill));
    els.skillManagerList.append(row);
  }
}

function renderTaskManager(lanes, tasks, channels, users, agents) {
  const orderedLanes = sortedTaskLanes(lanes);
  els.taskManagerCount.textContent = `${tasks.length} task${tasks.length === 1 ? "" : "s"}`;
  els.taskBoard.innerHTML = "";

  if (orderedLanes.length === 0) {
    const empty = document.createElement("div");
    empty.className = "task-empty";
    empty.innerHTML = "<strong>No lanes yet.</strong><span>Add a lane to start managing tasks.</span>";
    els.taskBoard.append(empty);
    return;
  }

  const channelById = new Map(channels.map((channel) => [channel.id, channel]));
  for (const lane of orderedLanes) {
    const laneTasks = tasks.filter((task) => task.laneId === lane.id);
    const laneEl = document.createElement("section");
    laneEl.className = "task-lane";
    laneEl.dataset.laneId = lane.id;
    laneEl.innerHTML = `
      <header class="task-lane-header">
        <div>
          <strong><span class="task-lane-status-icon" data-status-tone="${taskLaneTone(lane.name)}" aria-hidden="true"></span>${escapeHTML(lane.name)}</strong>
          <span>${laneTasks.length} task${laneTasks.length === 1 ? "" : "s"}</span>
        </div>
        <div class="task-lane-actions">
          <button class="task-lane-new" type="button" data-action="new-task">+</button>
        </div>
      </header>
      <div class="task-lane-body" data-lane-drop="${escapeHTML(lane.id)}"></div>`;

    const body = laneEl.querySelector(".task-lane-body");
    body.addEventListener("dragover", (event) => {
      event.preventDefault();
      laneEl.classList.add("drop-target");
    });
    body.addEventListener("dragleave", () => laneEl.classList.remove("drop-target"));
    body.addEventListener("drop", async (event) => {
      event.preventDefault();
      laneEl.classList.remove("drop-target");
      if (state.taskDragId) await moveTaskToLane(state.taskDragId, lane.id);
    });

    laneEl.querySelector("[data-action='new-task']").addEventListener("click", () => openTaskDialog(lane.id));

    if (laneTasks.length === 0) {
      const empty = document.createElement("div");
      empty.className = "task-lane-empty";
      empty.textContent = "Drop tasks here";
      body.append(empty);
    }

    for (const task of laneTasks) {
      const card = document.createElement("article");
      card.className = "task-card";
      card.classList.toggle("drawer-open", state.taskDrawerTaskId === task.id);
      card.draggable = true;
      card.dataset.taskId = task.id;
      const channel = task.channelId ? channelById.get(task.channelId) : null;
      const currentLane = orderedLanes.find((candidate) => candidate.id === task.laneId) || lane;
      const statusTone = taskLaneTone(currentLane?.name || "");
      card.innerHTML = `
        <div class="task-card-main">
          <strong>${escapeHTML(task.title)}</strong>
          ${task.description ? `<p>${escapeHTML(task.description)}</p>` : ""}
        </div>
        <div class="task-card-meta">
          <span>${escapeHTML(formatTaskTime(task.updatedAt || task.createdAt))}</span>
          ${task.workdir ? `<span title="${escapeHTML(task.workdir)}">cwd ${escapeHTML(compactPath(task.workdir))}</span>` : ""}
          ${channel ? `<span>#${escapeHTML(channel.name)}</span>` : "<span>No channel</span>"}
        </div>
        <div class="task-card-routing">
          <label class="task-owner-field">
            <span>Owner</span>
            <select class="task-owner-select" aria-label="Assign owner for ${escapeHTML(task.title)}">${taskAssigneeOptionsHTML(users, agents, task)}</select>
          </label>
          <label class="task-status-control" data-status-tone="${statusTone}">
            <span class="task-status-chip" aria-hidden="true">
              <span class="task-status-icon"></span>
              <span>${escapeHTML(currentLane?.name || "Unplanned")}</span>
            </span>
            <select class="task-status-select" aria-label="Move task ${escapeHTML(task.title)}">${taskLaneOptionsHTML(orderedLanes, task.laneId)}</select>
          </label>
        </div>
        <div class="task-card-controls">
          <button type="button" class="item-action visible" data-action="edit-task">Edit</button>
          <button type="button" class="item-action visible" data-action="discuss">Chat</button>
          <button type="button" class="item-delete visible" data-action="delete-task" aria-label="Delete task ${escapeHTML(task.title)}" title="Delete task">&times;</button>
        </div>`;
      card.addEventListener("click", (event) => {
        if (event.target.closest("button, select, textarea, input, label")) return;
        void openTaskChatDrawer(task);
      });
      card.addEventListener("dragstart", (event) => {
        state.taskDragId = task.id;
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("text/plain", task.id);
        card.classList.add("dragging");
      });
      card.addEventListener("dragend", () => {
        state.taskDragId = "";
        card.classList.remove("dragging");
      });
      card.querySelector(".task-status-select").addEventListener("change", (event) => {
        void moveTaskToLane(task.id, event.target.value);
      });
      card.querySelector(".task-owner-select").addEventListener("change", (event) => {
        void updateTaskAssignee(task.id, event.target.value);
      });
      card.querySelector("[data-action='edit-task']").addEventListener("click", () => openTaskDialog(task.laneId, task));
      card.querySelector("[data-action='discuss']").addEventListener("click", () => openTaskChatDrawer(task));
      card.querySelector("[data-action='delete-task']").addEventListener("click", () => deleteTask(task));
      body.append(card);
    }

    els.taskBoard.append(laneEl);
  }
  focusTaskCardIfNeeded();
}

function sortedTaskLanes(lanes) {
  return [...lanes].sort((a, b) => (a.position || 0) - (b.position || 0) || a.name.localeCompare(b.name));
}

function taskLaneOptionsHTML(lanes, selectedID) {
  return lanes
    .map((lane) => `<option value="${escapeHTML(lane.id)}" ${lane.id === selectedID ? "selected" : ""}>${escapeHTML(lane.name)}</option>`)
    .join("");
}

function taskLaneTone(name = "") {
  const normalized = name.trim().toLowerCase();
  if (normalized === "backlog") return "backlog";
  if (normalized === "todo" || normalized === "to do") return "todo";
  if (normalized === "doing" || normalized === "in progress") return "doing";
  if (normalized === "review" || normalized === "qa") return "review";
  if (normalized === "done" || normalized === "complete" || normalized === "completed") return "done";
  if (normalized === "unplanned" || normalized === "unplaned") return "unplanned";
  return "custom";
}

function visibleSidebarChannels(channels, tasks, lanes) {
  const doneTaskChannels = doneTaskChannelIds(tasks, lanes);
  return channels.filter((channel) => !channel.hidden && !doneTaskChannels.has(channel.id));
}

function doneTaskChannelIds(tasks, lanes) {
  const laneById = new Map((lanes || []).map((lane) => [lane.id, lane]));
  const doneChannels = new Set();
  for (const task of tasks || []) {
    if (!task.channelId) continue;
    const lane = laneById.get(task.laneId);
    if (taskLaneTone(lane?.name || "") === "done") {
      doneChannels.add(task.channelId);
    }
  }
  return doneChannels;
}

function taskAssigneeOptionsHTML(users, agents, task) {
  const selected = task.assigneeKind && task.assigneeId ? `${task.assigneeKind}:${task.assigneeId}` : "";
  const humanOptions = (users || [])
    .map((user) => taskAssigneeOptionHTML(`human:${user.id}`, user.name, selected))
    .join("");
  const agentOptions = (agents || [])
    .map((agent) => taskAssigneeOptionHTML(`agent:${agent.id}`, `${agent.name} · ${runtimeLabel(agent)}`, selected))
    .join("");
  return `
    <option value="" ${selected ? "" : "selected"}>Unassigned</option>
    ${humanOptions ? `<optgroup label="Humans">${humanOptions}</optgroup>` : ""}
    ${agentOptions ? `<optgroup label="Agents">${agentOptions}</optgroup>` : ""}`;
}

function taskAssigneeOptionHTML(value, label, selected) {
  return `<option value="${escapeHTML(value)}" ${value === selected ? "selected" : ""}>${escapeHTML(label)}</option>`;
}

function taskAssigneeLabel(task, users, agents) {
  if (!task?.assigneeKind || !task?.assigneeId) return "Unassigned";
  if (task.assigneeKind === "agent") {
    const agent = (agents || []).find((candidate) => candidate.id === task.assigneeId);
    return agent ? `Owner: ${agent.name}` : "Owner: missing agent";
  }
  const user = (users || []).find((candidate) => candidate.id === task.assigneeId);
  return user ? `Owner: ${user.name}` : "Owner: missing human";
}

function taskForChannel(channel, tasks = state.snapshot?.tasks || []) {
  if (!channel) return null;
  return tasks.find((task) => task.channelId === channel.id) || null;
}

function focusTaskCardIfNeeded() {
  const taskId = state.focusTaskId;
  if (!taskId) return;
  requestAnimationFrame(() => {
    const card = [...els.taskBoard.querySelectorAll(".task-card")].find((candidate) => candidate.dataset.taskId === taskId);
    if (!card) {
      state.focusTaskId = "";
      return;
    }
    card.scrollIntoView({ block: "center", inline: "center", behavior: "smooth" });
    card.classList.add("focused");
    window.setTimeout(() => card.classList.remove("focused"), 1800);
    state.focusTaskId = "";
  });
}

function renderAgentSkillLibrary() {
  const skills = state.snapshot?.skills || [];
  els.agentSkillLibrary.innerHTML = "";
  if (skills.length === 0) {
    const empty = document.createElement("p");
    empty.className = "muted";
    empty.textContent = "No Skill Center skills yet.";
    els.agentSkillLibrary.append(empty);
    return;
  }
  for (const skill of skills) {
    const label = document.createElement("label");
    label.className = "agent-skill-choice";
    label.innerHTML = `
      <input type="checkbox" value="${escapeHTML(skill.id)}" />
      <span>
        <strong>${escapeHTML(skill.name)}</strong>
        <small>${escapeHTML(skill.source || "manual import")}${skill.tags?.length ? ` · ${skill.tags.map((tag) => `#${tag}`).join(" ")}` : ""}</small>
      </span>`;
    els.agentSkillLibrary.append(label);
  }
}

function selectedAgentSkillIDs() {
  return [...els.agentSkillLibrary.querySelectorAll("input[type='checkbox']:checked")].map((input) => input.value);
}

function renderSkillTagFilter(skills) {
  const tags = [...new Set(skills.flatMap((skill) => skill.tags || []))].sort();
  if (state.skillTagFilter && !tags.includes(state.skillTagFilter)) {
    state.skillTagFilter = "";
  }
  els.skillTagFilter.innerHTML = "";
  const all = document.createElement("option");
  all.value = "";
  all.textContent = "All tags";
  els.skillTagFilter.append(all);
  for (const tag of tags) {
    const option = document.createElement("option");
    option.value = tag;
    option.textContent = `#${tag}`;
    els.skillTagFilter.append(option);
  }
  els.skillTagFilter.value = state.skillTagFilter;
}

function skillUsage(skill, agents) {
  const users = agents.filter((agent) => agentHasSkill(agent, skill.id)).map((agent) => agent.name);
  if (users.length === 0) return "Not attached";
  if (users.length <= 2) return `Used by ${users.join(", ")}`;
  return `Used by ${users.slice(0, 2).join(", ")} +${users.length - 2}`;
}

function agentHasSkill(agent, skillID) {
  return (agent.skillIds || []).includes(skillID) || (agent.skills || []).some((skill) => skill.id === skillID);
}

function skillExcerpt(content = "") {
  const compacted = cleanMarkdownInline(stripSkillFileLabel(content).replace(/\s+/g, " "));
  return compacted.slice(0, 180) || "No content preview.";
}

function skillTagsHTML(skill) {
  const tags = skill.tags || [];
  if (tags.length === 0) return "";
  return `<span class="skill-tags">${tags.map((tag) => `<span class="skill-tag">#${escapeHTML(tag)}</span>`).join("")}</span>`;
}

function openSkillContent(skill, meta = "Library skill") {
  els.markdownDialogTitle.textContent = skill.name || "Imported skill";
  const tags = (skill.tags || []).map((tag) => `#${tag}`).join(" ");
  els.markdownDialogMeta.textContent = `${meta} · ${skill.source || "manual import"}${tags ? ` · ${tags}` : ""}`;
  els.markdownDialogBody.innerHTML = renderMarkdown(normalizeSkillPreviewMarkdown(skill.content || ""));
  els.markdownDialogBody.scrollTop = 0;
  els.markdownDialog.showModal();
}

function renderSkillDialog() {
  if (state.skillDialogMode === "global") {
    els.skillDialogTitle.textContent = "Create Skill";
    els.skillAgentName.textContent = "Global skill library";
    els.skillList.innerHTML = "";
    els.skillList.hidden = true;
    els.skillAttachRow.hidden = true;
    updateSkillCreateModeUI();
    return;
  }

  const agent = currentSkillAgent();
  els.skillDialogTitle.textContent = "Manage Agent Skills";
  els.skillAgentName.textContent = agent ? `${agent.name} · ${runtimeLabel(agent)}` : "";
  els.skillList.hidden = false;
  els.skillList.innerHTML = "";
  if (!agent) return;
  renderSkillAttachOptions(agent);
  updateSkillCreateModeUI();
  const skills = agent.skills || [];
  if (skills.length === 0) {
    const empty = document.createElement("p");
    empty.className = "muted";
    empty.textContent = "No skills attached yet.";
    els.skillList.append(empty);
    return;
  }
  for (const skill of skills) {
    const row = document.createElement("div");
    row.className = "skill-row";
    const copy = document.createElement("div");
    copy.className = "skill-copy";
    copy.innerHTML = `<strong>${escapeHTML(skill.name)}</strong><span>${escapeHTML(skill.source || "manual import")} · ${skill.content ? `${skill.content.length} chars` : "empty"}</span>${skillTagsHTML(skill)}`;
    const deleteButton = document.createElement("button");
    deleteButton.type = "button";
    deleteButton.className = "item-delete visible";
    deleteButton.title = `Detach skill ${skill.name}`;
    deleteButton.setAttribute("aria-label", `Detach skill ${skill.name}`);
    deleteButton.textContent = "x";
    deleteButton.addEventListener("click", () => deleteAgentSkill(agent, skill));
    row.append(copy, deleteButton);
    els.skillList.append(row);
  }
}

function renderSkillAttachOptions(agent) {
  const attached = new Set((agent.skills || []).map((skill) => skill.id));
  const available = (state.snapshot?.skills || []).filter((skill) => !attached.has(skill.id));
  els.skillAttachSelect.innerHTML = "";
  for (const skill of available) {
    const option = document.createElement("option");
    option.value = skill.id;
    const tags = (skill.tags || []).map((tag) => `#${tag}`).join(" ");
    option.textContent = `${skill.name} · ${skill.source || "manual import"}${tags ? ` · ${tags}` : ""}`;
    els.skillAttachSelect.append(option);
  }
  if (available.length === 0) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No unattached skills";
    els.skillAttachSelect.append(option);
  }
  els.skillImport.disabled = state.skillCreateMode === "attach" && available.length === 0;
}

function currentSkillAgent() {
  if (!state.snapshot || !state.skillAgentId) return null;
  return (state.snapshot.agents || []).find((agent) => agent.id === state.skillAgentId) || null;
}

async function deleteAgentSkill(agent, skill) {
  if (!window.confirm(`Detach skill ${skill.name} from ${agent.name}? The skill remains in Skill Center.`)) return;
  try {
    await api(`/api/agents/${encodeURIComponent(agent.id)}/skills/${encodeURIComponent(skill.id)}`, { method: "DELETE" });
    state.snapshot = await api("/api/state");
    render();
    if (els.skillDialog.open) renderSkillDialog();
  } catch (error) {
    alert(error.message);
  }
}

async function deleteGlobalSkill(skill) {
  if (!window.confirm(`Delete skill ${skill.name} from Skill Center? This also detaches it from every agent.`)) return;
  try {
    await api(`/api/skills/${encodeURIComponent(skill.id)}`, { method: "DELETE" });
    state.snapshot = await api("/api/state");
    render();
    if (els.skillDialog.open) renderSkillDialog();
  } catch (error) {
    alert(error.message);
  }
}

async function deleteUser(user) {
  if (!window.confirm(`Delete ${user.name}? Existing messages from this human stay in the channel history.`)) return;
  try {
    await api(`/api/users/${encodeURIComponent(user.id)}`, { method: "DELETE" });
  } catch (error) {
    alert(error.message);
  }
}

function insertMention(name) {
  const suffix = els.input.value && !els.input.value.endsWith(" ") ? " " : "";
  els.input.value += `${suffix}${displayMentionHandle(name)} `;
  els.input.focus();
}

function insertTextAtCursor(text) {
  const start = els.input.selectionStart || 0;
  const end = els.input.selectionEnd || start;
  const before = els.input.value.slice(0, start);
  const after = els.input.value.slice(end);
  els.input.value = `${before}${text}${after}`;
  const cursor = start + text.length;
  els.input.setSelectionRange(cursor, cursor);
  els.input.focus();
}

function insertTaskChatMention(name) {
  const suffix = els.taskChatInput.value && !els.taskChatInput.value.endsWith(" ") ? " " : "";
  els.taskChatInput.value += `${suffix}${displayMentionHandle(name)} `;
  state.taskDrawerDraft = els.taskChatInput.value;
  els.taskChatInput.focus();
}

function insertTaskChatTextAtCursor(text) {
  const start = els.taskChatInput.selectionStart || 0;
  const end = els.taskChatInput.selectionEnd || start;
  const before = els.taskChatInput.value.slice(0, start);
  const after = els.taskChatInput.value.slice(end);
  els.taskChatInput.value = `${before}${text}${after}`;
  const cursor = start + text.length;
  els.taskChatInput.setSelectionRange(cursor, cursor);
  state.taskDrawerDraft = els.taskChatInput.value;
  els.taskChatInput.focus();
}

function sanitizeAgentNameField() {
  const start = els.agentName.selectionStart || 0;
  const before = els.agentName.value.slice(0, start);
  const normalizedBefore = normalizeAgentNameInput(before);
  const normalized = normalizeAgentNameInput(els.agentName.value);
  if (normalized === els.agentName.value) return;
  els.agentName.value = normalized;
  const cursor = normalizedBefore.length;
  els.agentName.setSelectionRange(cursor, cursor);
}

function normalizeAgentNameInput(value = "") {
  return value.replace(/\s+/g, "");
}

function showHoverTooltip(target) {
  const text = target.dataset.tooltip;
  if (!text) return;
  hoverTooltipTarget = target;
  hoverTooltip.textContent = text;
  hoverTooltip.hidden = false;
  positionHoverTooltip(target);
}

function positionHoverTooltip(target) {
  const rect = target.getBoundingClientRect();
  const width = Math.min(260, window.innerWidth - 24);
  const left = Math.min(Math.max(12, rect.left), Math.max(12, window.innerWidth - width - 12));
  const top = Math.min(rect.bottom + 8, Math.max(12, window.innerHeight - hoverTooltip.offsetHeight - 12));
  hoverTooltip.style.left = `${left}px`;
  hoverTooltip.style.top = `${top}px`;
}

function hideHoverTooltip() {
  hoverTooltipTarget = null;
  hoverTooltip.hidden = true;
}

function showProfilePopover(target) {
  const profile = participantProfile(target);
  if (!profile) return;
  profilePopoverTarget = target;
  profilePopover.innerHTML = profilePopoverHTML(profile);
  profilePopover.hidden = false;
  positionFloatingCard(profilePopover, target, 12);
}

function hideProfilePopover() {
  profilePopoverTarget = null;
  profilePopover.hidden = true;
}

function positionFloatingCard(card, target, gap = 8) {
  const rect = target.getBoundingClientRect();
  const width = Math.min(card.offsetWidth || 320, window.innerWidth - 24);
  const rightSide = rect.right + gap;
  const left = rightSide + width <= window.innerWidth - 12
    ? rightSide
    : Math.min(Math.max(12, rect.left), Math.max(12, window.innerWidth - width - 12));
  const top = Math.min(Math.max(12, rect.top), Math.max(12, window.innerHeight - card.offsetHeight - 12));
  card.style.left = `${left}px`;
  card.style.top = `${top}px`;
}

function participantProfile(target) {
  if (!state.snapshot) return null;
  const kind = target.dataset.profileKind;
  const id = target.dataset.profileId;
  if (kind === "agent") {
    const agent = (state.snapshot.agents || []).find((candidate) => candidate.id === id);
    if (agent) return { kind: "agent", value: agent };
  }
  if (kind === "human") {
    const user = (state.snapshot.users || []).find((candidate) => candidate.id === id);
    if (user) return { kind: "human", value: user };
  }
  if (!target.dataset.profileName) return null;
  return {
    kind,
    value: {
      id,
      name: target.dataset.profileName,
      color: kind === "human" ? "#2563eb" : "#64748b",
    },
  };
}

function profilePopoverHTML(profile) {
  if (profile.kind === "agent") return agentProfileHTML(profile.value);
  return humanProfileHTML(profile.value);
}

function agentProfileHTML(agent) {
  const skills = agent.skills || [];
  const capabilities = agent.capabilities || [];
  const skillPreview = skills.slice(0, 3).map((skill) => skill.name).join(", ");
  const capabilityPreview = capabilities.slice(0, 4).join(", ");
  return `
    <div class="profile-head">
      <span class="avatar profile-avatar" style="background:${agent.color || "#64748b"}">${initials(agent.name)}</span>
      <span>
        <strong>${escapeHTML(agent.name)}</strong>
        <small>Agent · ${escapeHTML(agent.status || "idle")}</small>
      </span>
    </div>
    <div class="profile-grid">
      <span>Runtime</span><strong>${escapeHTML(runtimeLabel(agent))}</strong>
      <span>Skills</span><strong>${skills.length}${skillPreview ? ` · ${escapeHTML(skillPreview)}` : ""}</strong>
      <span>Memory</span><strong>${(agent.memory || []).length} items</strong>
      <span>System</span><strong>${agent.systemPrompt ? "prompt configured" : "default prompt"}</strong>
    </div>
    ${agent.persona ? `<p class="profile-note">${escapeHTML(agent.persona)}</p>` : ""}
    ${capabilityPreview ? `<div class="profile-tags">${capabilities.slice(0, 4).map((capability) => `<span>${escapeHTML(capability)}</span>`).join("")}</div>` : ""}
  `;
}

function humanProfileHTML(user) {
  const currentUserId = state.snapshot?.currentUserId || "usr_you";
  const role = user.id === currentUserId ? "Current human" : "Human participant";
  return `
    <div class="profile-head">
      <span class="avatar profile-avatar" style="background:${user.color || "#2563eb"}">${initials(user.name)}</span>
      <span>
        <strong>${escapeHTML(user.name)}</strong>
        <small>${role}</small>
      </span>
    </div>
    <div class="profile-grid">
      <span>Role</span><strong>${role}</strong>
      <span>ID</span><strong>${escapeHTML(user.id || "unknown")}</strong>
      <span>Status</span><strong>${user.id === currentUserId ? "active in this workspace" : "registered"}</strong>
    </div>
  `;
}

function populateModelOptions(runtime) {
  const options = runtimeModels[runtime] || runtimeModels.codex;
  els.agentModel.innerHTML = "";
  for (const [value, label] of options) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    els.agentModel.append(option);
  }
  updateCustomModelVisibility();
}

function setAgentModelValue(runtime, model = "") {
  populateModelOptions(runtime);
  const hasOption = [...els.agentModel.options].some((option) => option.value === model);
  if (hasOption) {
    els.agentModel.value = model;
    els.agentModelCustom.value = "";
  } else {
    els.agentModel.value = "__custom";
    els.agentModelCustom.value = model;
  }
  updateCustomModelVisibility();
}

function resetAgentDialog() {
  state.editingAgentId = "";
  els.agentDialogTitle.textContent = "Create Agent";
  els.agentSubmit.textContent = "Create";
  els.agentName.value = "";
  els.agentPersona.value = "";
  els.agentSystemPrompt.value = "";
  els.agentRuntime.value = "codex";
  els.agentModelCustom.value = "";
  els.agentSkills.value = "";
  els.agentSkillsRow.hidden = false;
  els.agentSkillPicker.hidden = false;
  renderAgentSkillLibrary();
  populateModelOptions("codex");
}

function parseInitialSkills(text = "") {
  return text
    .split(/^---+$/m)
    .map((block) => block.trim())
    .filter(Boolean)
    .map((block) => {
      const lines = block.split("\n");
      const firstLine = (lines.find((line) => line.trim()) || "Imported skill").trim();
      const heading = firstLine.match(/^#{1,6}\s+(.+)$/);
      return {
        name: cleanMarkdownInline(heading?.[1] || firstLine).slice(0, 80) || "Imported skill",
        source: "create-agent",
        content: block,
      };
    });
}

function parseSkillTags(text = "") {
  const tags = [];
  for (const raw of text.split(/[,\n]/)) {
    const tag = raw.trim().replace(/^#/, "").toLowerCase().replace(/\s+/g, "-").replace(/^-+|-+$/g, "");
    if (tag && !tags.includes(tag)) tags.push(tag.slice(0, 32).replace(/-+$/g, ""));
  }
  return tags.filter(Boolean);
}

function isSupportedCloudSkillURL(value = "") {
  const source = extractCloudSkillURL(value);
  if (!source) return false;
  try {
    const url = new URL(source);
    if (url.protocol !== "http:" && url.protocol !== "https:") return false;
    return ["skills.sh", "www.skills.sh", "github.com", "raw.githubusercontent.com"].includes(url.hostname.toLowerCase());
  } catch {
    return false;
  }
}

function extractCloudSkillURL(value = "") {
  const source = value.trim();
  if (!source) return "";
  if (source.startsWith("http://") || source.startsWith("https://")) return source.replace(/^['"]|['"]$/g, "");
  if (!source.startsWith("npx ") && !source.startsWith("npx\t") && source !== "npx") return source;
  const matches = source.match(/https?:\/\/[^\s"'<>]+/g) || [];
  for (const match of matches) {
    const candidate = match.replace(/^['"]|['"]$/g, "").replace(/[.,)]+$/g, "");
    try {
      const url = new URL(candidate);
      if (["skills.sh", "www.skills.sh", "github.com", "raw.githubusercontent.com"].includes(url.hostname.toLowerCase())) {
        return candidate;
      }
    } catch {
      // Keep scanning the command for another URL.
    }
  }
  return "";
}

function updateCustomModelVisibility() {
  els.agentModelCustomRow.hidden = els.agentModel.value !== "__custom";
}

function runtimeLabel(agent) {
  const runtime = agent.runtime || "codex";
  const model = agent.model || "default";
  return `${runtime}/${model}`;
}

function channelDefaultAgentId(channel, agents) {
  if (!channel) return "";
  if (agents.some((agent) => agent.id === channel.defaultAgentId)) {
    return channel.defaultAgentId;
  }
  const byId = new Set(agents.map((agent) => agent.id));
  for (const memberId of channel.memberIds || []) {
    if (byId.has(memberId)) return memberId;
  }
  return agents[0]?.id || "";
}

function initials(name = "?") {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join("");
}

function formatTime(value) {
  if (!value) return "";
  const date = new Date(value);
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function formatTaskTime(value) {
  if (!value) return "No timestamp";
  const date = new Date(value);
  return date.toLocaleDateString([], { month: "short", day: "numeric" });
}

function compactPath(value = "") {
  const trimmed = value.trim();
  if (trimmed.length <= 34) return trimmed;
  const parts = trimmed.split(/[\\/]+/).filter(Boolean);
  if (parts.length >= 2) {
    return `.../${parts.slice(-2).join("/")}`;
  }
  return `...${trimmed.slice(-31)}`;
}

function linkMentions(text) {
  return text.replace(mentionTokenRegex(), (full, token) => {
    const participant = participantForMention(token);
    if (!participant) return `<strong>${full}</strong>`;
    const display = `@${escapeHTML(decodeMentionToken(token))}`;
    return `<strong class="mention-profile profile-anchor" ${profileAnchorAttrs(participant.kind, participant.value)} tabindex="0">${display}</strong>`;
  });
}

function participantForMention(token = "") {
  if (!state.snapshot) return null;
  const normalized = mentionKey(token);
  const agent = (state.snapshot.agents || []).find((candidate) => mentionKey(candidate.name) === normalized || mentionKey(candidate.id) === normalized);
  if (agent) return { kind: "agent", value: agent };
  const user = (state.snapshot.users || []).find((candidate) => mentionKey(candidate.name) === normalized || mentionKey(candidate.id) === normalized);
  if (user) return { kind: "human", value: user };
  return null;
}

function mentionTokenRegex() {
  return /@([^\s@<>()\[\]{}.,，。:：;；!?！？]+)/g;
}

function mentionHandle(name = "") {
  const value = name.trim();
  if (!value) return "@";
  if (/[\s@<>()\[\]{}.,，。:：;；!?！？%]/.test(value)) return `@${encodeURIComponent(value)}`;
  return `@${value}`;
}

function displayMentionHandle(name = "") {
  const value = name.trim();
  return value ? `@${value}` : "@";
}

function encodeMessageMentionsForWire(text = "") {
  if (!state.snapshot || !text) return text;
  const participants = [...(state.snapshot.users || []), ...(state.snapshot.agents || [])]
    .filter((participant) => participant.name && mentionHandle(participant.name) !== displayMentionHandle(participant.name))
    .sort((a, b) => b.name.length - a.name.length);
  let next = text;
  for (const participant of participants) {
    const raw = displayMentionHandle(participant.name);
    const encoded = mentionHandle(participant.name);
    next = next.replace(rawMentionRegex(raw), (_match, prefix) => `${prefix}${encoded}`);
  }
  return next;
}

function rawMentionRegex(rawHandle) {
  return new RegExp(`(^|[\\s([{])${escapeRegExp(rawHandle)}(?=$|[\\s<>()\\[\\]{}.,，。:：;；!?！？])`, "g");
}

function escapeRegExp(value = "") {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function mentionKey(value = "") {
  return decodeMentionToken(value.replace(/^@/, "")).trim().toLowerCase();
}

function decodeMentionToken(value = "") {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function profileAnchorAttrs(kind, value) {
  return `data-profile-kind="${escapeHTML(kind)}" data-profile-id="${escapeHTML(value.id || "")}" data-profile-name="${escapeHTML(value.name || "")}"`;
}

function normalizeSkillPreviewMarkdown(text = "") {
  const lines = stripSkillFileLabel(text).replace(/\r\n/g, "\n").split("\n");
  const out = [];
  let index = 0;

  while (index < lines.length) {
    const line = lines[index];
    if (line.trim().startsWith("```")) {
      out.push(line);
      index += 1;
      while (index < lines.length) {
        out.push(lines[index]);
        if (lines[index].trim().startsWith("```")) {
          index += 1;
          break;
        }
        index += 1;
      }
      continue;
    }

    if (startsTreeBlock(lines, index)) {
      index = appendAutoFencedBlock(out, lines, index, isTreeBlockContinuation);
      continue;
    }

    if (startsLooseCodeBlock(lines, index)) {
      index = appendAutoFencedBlock(out, lines, index, isLooseCodeBlockContinuation);
      continue;
    }

    out.push(line);
    index += 1;
  }

  return out.join("\n");
}

function stripSkillFileLabel(text = "") {
  return text.replace(/^\s*>?\s*SKILL\.md\s*\n+/i, "");
}

function normalizeChatBlockMarkdown(text = "") {
  return text.replace(/\r\n/g, "\n").trim();
}

function appendAutoFencedBlock(out, lines, start, continues) {
  const block = [];
  let index = start;
  while (index < lines.length && continues(lines, index)) {
    block.push(lines[index]);
    index += 1;
  }
  while (block.length && !block[0].trim()) block.shift();
  while (block.length && !block[block.length - 1].trim()) block.pop();
  if (block.length === 0) return index;
  if (out.length && out[out.length - 1].trim()) out.push("");
  out.push("```text", ...block, "```");
  if (index < lines.length && lines[index].trim()) out.push("");
  return index;
}

function startsTreeBlock(lines, index) {
  const trimmed = lines[index]?.trim() || "";
  const next = lines[index + 1] || "";
  return isTreeLine(lines[index] || "") || (isDirectoryRootLine(trimmed) && isTreeLine(next));
}

function isTreeBlockContinuation(lines, index) {
  const line = lines[index] || "";
  const trimmed = line.trim();
  if (!trimmed) return true;
  return isTreeLine(line) || isDirectoryRootLine(trimmed);
}

function isTreeLine(line = "") {
  return /^\s*[│├└╭╰┌└┬┴─|]/.test(line);
}

function isDirectoryRootLine(trimmed = "") {
  return /^[\w.-]+\/(?:\s+#.*)?$/.test(trimmed);
}

function startsLooseCodeBlock(lines, index) {
  if (!isLooseCodeStart(lines[index] || "")) return false;
  let codeSignals = 0;
  for (let offset = 0; offset < 8 && index + offset < lines.length; offset += 1) {
    if (isLooseCodeLine(lines[index + offset] || "")) codeSignals += 1;
  }
  return codeSignals >= 3;
}

function isLooseCodeBlockContinuation(lines, index) {
  const line = lines[index] || "";
  const trimmed = line.trim();
  if (!trimmed) return true;
  if (/^#{2,6}\s+/.test(trimmed)) return false;
  if (/^[-*]\s+\S/.test(trimmed) || /^\d+\.\s+\S/.test(trimmed)) return false;
  return true;
}

function isLooseCodeStart(line = "") {
  const trimmed = line.trim();
  return isCodeFileComment(trimmed) || /^(from|import|class|def|async\s+def|@dataclass|@abstractmethod)\b/.test(trimmed);
}

function isLooseCodeLine(line = "") {
  const trimmed = line.trim();
  if (!trimmed) return false;
  return (
    isCodeFileComment(trimmed) ||
    /^(from|import|class|def|async\s+def|if|for|while|try|except|with|async\s+with|return|raise|await|@dataclass|@abstractmethod|@router)\b/.test(trimmed) ||
    /^[A-Za-z_][\w.]*\s*[:=]/.test(trimmed) ||
    /^self\./.test(trimmed) ||
    /^[})\]]/.test(trimmed)
  );
}

function isCodeFileComment(trimmed = "") {
  return /^#\s*\S+\.(py|go|js|jsx|ts|tsx|rs|java|rb|php|swift|kt|cs|cpp|c|h|sql|ya?ml|json|toml|md)\b/i.test(trimmed);
}

function renderMarkdown(text = "") {
  const lines = text.split("\n");
  const html = [];
  let inCode = false;
  let codeLines = [];
  let listType = "";
  let paragraphLines = [];

  const closeParagraph = () => {
    if (paragraphLines.length) {
      html.push(`<p>${inlineMarkdown(paragraphLines.join(" "))}</p>`);
      paragraphLines = [];
    }
  };
  const closeList = () => {
    if (listType) {
      html.push(`</${listType}>`);
      listType = "";
    }
  };
  const openList = (type) => {
    if (listType === type) return;
    closeParagraph();
    closeList();
    html.push(`<${type}>`);
    listType = type;
  };
  const closeCode = () => {
    html.push(`<pre><code>${escapeHTML(codeLines.join("\n"))}</code></pre>`);
    codeLines = [];
    inCode = false;
  };

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    if (line.trim().startsWith("```")) {
      closeParagraph();
      closeList();
      if (inCode) closeCode();
      else inCode = true;
      continue;
    }
    if (inCode) {
      codeLines.push(line);
      continue;
    }

    const trimmed = line.trim();
    if (!trimmed) {
      closeParagraph();
      if (listType && lineListType(nextNonEmptyMarkdownLine(lines, index + 1)) === listType) continue;
      closeList();
      continue;
    }

    const heading = trimmed.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      closeParagraph();
      closeList();
      const level = Math.min(heading[1].length + 1, 4);
      html.push(`<h${level}>${inlineMarkdown(heading[2])}</h${level}>`);
      continue;
    }

    const itemType = lineListType(trimmed);
    if (itemType) {
      openList(itemType);
      html.push(`<li>${inlineMarkdown(listItemText(trimmed))}</li>`);
      continue;
    }

    if (trimmed.startsWith(">")) {
      closeParagraph();
      closeList();
      html.push(`<blockquote>${inlineMarkdown(trimmed.replace(/^>\s?/, ""))}</blockquote>`);
      continue;
    }

    if (/^\|.+\|$/.test(trimmed)) {
      closeParagraph();
      closeList();
      html.push(`<pre class="markdown-table">${escapeHTML(trimmed)}</pre>`);
      continue;
    }

    closeList();
    paragraphLines.push(trimmed);
  }

  closeParagraph();
  closeList();
  if (inCode) closeCode();
  return html.join("");
}

function nextNonEmptyMarkdownLine(lines, start) {
  for (let index = start; index < lines.length; index += 1) {
    const trimmed = lines[index].trim();
    if (trimmed) return trimmed;
  }
  return "";
}

function lineListType(line = "") {
  const trimmed = line.trim();
  if (/^[-*]\s+\S/.test(trimmed)) return "ul";
  if (/^\d+\.\s+\S/.test(trimmed)) return "ol";
  return "";
}

function listItemText(line = "") {
  return line.trim().replace(/^[-*]\s+/, "").replace(/^\d+\.\s+/, "");
}

function inlineMarkdown(text = "") {
  return linkMentions(escapeHTML(text))
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/`([^`]+)`/g, "<code>$1</code>");
}

function escapeHTML(value = "") {
  return value.replace(/[&<>"']/g, (char) => {
    const map = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;" };
    return map[char];
  });
}

load().catch((error) => {
  document.body.innerHTML = `<pre>${escapeHTML(error.message)}</pre>`;
});

applyTheme(state.theme);
applyChatFontSize(state.chatFontSize);
populateModelOptions("codex");
