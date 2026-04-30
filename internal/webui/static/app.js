const state = {
  snapshot: null,
  view: "channel",
  channelId: "chan_general",
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
};

const markdownDocumentStartMarker = "<<<MARKDOWN_DOCUMENT>>>";
const markdownDocumentEndMarker = "<<<END_MARKDOWN_DOCUMENT>>>";

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
  channelList: document.querySelector("#channel-list"),
  userList: document.querySelector("#user-list"),
  agentList: document.querySelector("#agent-list"),
  openSkills: document.querySelector("#open-skills"),
  roomEyebrow: document.querySelector("#room-eyebrow"),
  roomName: document.querySelector("#room-name"),
  defaultAgentControl: document.querySelector(".default-agent-control"),
  daemonChip: document.querySelector("#daemon-chip"),
  daemonCount: document.querySelector("#daemon-count"),
  messages: document.querySelector("#messages"),
  skillManager: document.querySelector("#skill-manager"),
  skillManagerCount: document.querySelector("#skill-manager-count"),
  skillManagerAdd: document.querySelector("#skill-manager-add"),
  skillSearch: document.querySelector("#skill-search"),
  skillTagFilter: document.querySelector("#skill-tag-filter"),
  skillManagerList: document.querySelector("#skill-manager-list"),
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
  agentName: document.querySelector("#agent-name"),
  agentPersona: document.querySelector("#agent-persona"),
  agentSystemPrompt: document.querySelector("#agent-system-prompt"),
  agentRuntime: document.querySelector("#agent-runtime"),
  agentModel: document.querySelector("#agent-model"),
  agentModelCustom: document.querySelector("#agent-model-custom"),
  agentModelCustomRow: document.querySelector("#agent-model-custom-row"),
  agentSkills: document.querySelector("#agent-skills"),
  agentSkillLibrary: document.querySelector("#agent-skill-library"),
  userDialog: document.querySelector("#user-dialog"),
  userName: document.querySelector("#user-name"),
  skillDialog: document.querySelector("#skill-dialog"),
  skillDialogTitle: document.querySelector("#skill-dialog-title"),
  skillAgentName: document.querySelector("#skill-agent-name"),
  skillList: document.querySelector("#skill-list"),
  skillAttachRow: document.querySelector("#skill-attach-row"),
  skillAttachSelect: document.querySelector("#skill-attach-select"),
  skillAttach: document.querySelector("#skill-attach"),
  skillModeButtons: document.querySelectorAll("[data-skill-mode]"),
  skillName: document.querySelector("#skill-name"),
  skillNameLabel: document.querySelector("#skill-name-label"),
  skillSource: document.querySelector("#skill-source"),
  skillSourceLabel: document.querySelector("#skill-source-label"),
  skillTags: document.querySelector("#skill-tags"),
  skillLocalFields: document.querySelector("#skill-local-fields"),
  skillFile: document.querySelector("#skill-file"),
  skillContent: document.querySelector("#skill-content"),
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
  const { channels, users, agents, daemons, events } = state.snapshot;
  if (!channels.some((channel) => channel.id === state.channelId)) {
    state.channelId = channels[0]?.id || "";
  }
  const current = channels.find((channel) => channel.id === state.channelId);
  const isSkillView = state.view === "skills";
  els.roomEyebrow.textContent = isSkillView ? "Management" : "Channel";
  els.roomName.textContent = isSkillView ? "Skill Center" : current ? `#${current.name}` : "#channel";
  els.defaultAgentControl.hidden = isSkillView;
  els.messages.hidden = isSkillView;
  els.composer.hidden = isSkillView;
  els.skillManager.hidden = !isSkillView;
  els.openSkills.classList.toggle("active", isSkillView);

  renderChannels(channels);
  renderUsers(users || []);
  renderAgents(agents);
  if (isSkillView) {
    renderSkillManager(state.snapshot.skills || [], agents);
  } else {
    renderMessages();
    renderMentions(availableMentionAgents(current, agents));
  }
  renderDaemon(daemons);
  renderChannelSettings(current, agents);
  renderAssign(agents);
  renderEvents(events || []);
  if (state.skillAgentId && els.skillDialog.open) renderSkillDialog();
}

function renderChannels(channels) {
  els.channelList.innerHTML = "";
  for (const channel of channels) {
    const row = document.createElement("div");
    row.className = "nav-row";
    const button = document.createElement("button");
    button.className = `nav-item ${state.view === "channel" && channel.id === state.channelId ? "active" : ""}`;
    button.title = channel.topic ? `#${channel.name} - ${channel.topic}` : `#${channel.name}`;
    button.dataset.tooltip = channel.topic ? `#${channel.name}\n${channel.topic}` : `#${channel.name}`;
    button.innerHTML = `<span class="hash">#</span><span><strong>${escapeHTML(channel.name)}</strong><span class="nav-topic">${escapeHTML(channel.topic || "")}</span></span>`;
    button.addEventListener("click", () => {
      state.view = "channel";
      state.channelId = channel.id;
      render();
    });
    const deleteButton = document.createElement("button");
    deleteButton.type = "button";
    deleteButton.className = "item-delete";
    deleteButton.title = `Delete #${channel.name}`;
    deleteButton.setAttribute("aria-label", `Delete channel ${channel.name}`);
    deleteButton.textContent = "x";
    deleteButton.addEventListener("click", () => deleteChannel(channel));
    row.append(button, deleteButton);
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
  for (const agent of agents) {
    const row = document.createElement("div");
    row.className = "agent-row";
    const button = document.createElement("button");
    button.className = "agent-item";
    const skillCount = (agent.skills || []).length;
    const skillMeta = ` · ${skillCount} skill${skillCount === 1 ? "" : "s"}`;
    const promptMeta = agent.systemPrompt ? " · system prompt" : "";
    const meta = `${agent.status} · ${runtimeLabel(agent)}${skillMeta}${promptMeta} · ${agent.persona}`;
    button.title = `${agent.name} - ${meta}`;
    button.dataset.profileKind = "agent";
    button.dataset.profileId = agent.id;
    button.dataset.profileName = agent.name;
    button.innerHTML = `<span class="avatar" style="background:${agent.color || "#2563eb"}">${initials(agent.name)}</span><span><strong>${escapeHTML(agent.name)}</strong><span class="agent-meta">${escapeHTML(meta)}</span></span>`;
    button.addEventListener("click", () => {
      insertMention(agent.name);
    });
    const skillButton = document.createElement("button");
    skillButton.type = "button";
    skillButton.className = "item-action";
    skillButton.title = `Import skill into ${agent.name}`;
    skillButton.setAttribute("aria-label", `Import skill into ${agent.name}`);
    skillButton.textContent = "Skill";
    skillButton.addEventListener("click", () => openSkillDialog(agent));
    const deleteButton = document.createElement("button");
    deleteButton.type = "button";
    deleteButton.className = "item-delete";
    deleteButton.title = `Delete ${agent.name}`;
    deleteButton.setAttribute("aria-label", `Delete agent ${agent.name}`);
    deleteButton.textContent = "x";
    deleteButton.addEventListener("click", () => deleteAgent(agent));
    row.append(button, skillButton, deleteButton);
    els.agentList.append(row);
  }
}

function renderMessages() {
  const { messages, agents, users } = state.snapshot;
  const byAgent = new Map(agents.map((agent) => [agent.id, agent]));
  const byUser = new Map((users || []).map((user) => [user.id, user]));
  const visible = messages.filter((message) => message.channelId === state.channelId);
  els.messages.innerHTML = "";
  for (const message of visible) {
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
    els.messages.append(item);
  }
  els.messages.scrollTop = els.messages.scrollHeight;
}

function renderMessageContent(message) {
  if (!isMarkdownDocumentMessage(message)) {
    return `<div class="message-text">${linkMentions(escapeHTML(message.text))}</div>`;
  }
  const parts = markdownDocumentParts(message);
  const title = markdownDocumentTitle(parts.document);
  const stats = markdownStats(parts.document);
  const excerpt = markdownExcerpt(parts.document, title);
  const before = parts.before ? `<div class="message-text">${linkMentions(escapeHTML(parts.before))}</div>` : "";
  const after = parts.after ? `<div class="message-text">${linkMentions(escapeHTML(parts.after))}</div>` : "";
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
  for (const match of text.matchAll(/@([a-z0-9][a-z0-9_-]{0,40})/gi)) {
    tokens.add(match[1].toLowerCase());
  }
  return tokens;
}

function agentMentionToken(value = "") {
  return value.trim().toLowerCase().replace(/\s+/g, "-");
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
  if (/^@[A-Za-z0-9_-]+\b/.test(trimmed)) return true;
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
  els.markdownDialog.showModal();
}

function renderMentions(agents) {
  els.mentionRow.innerHTML = "";
  for (const agent of agents) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "mention-button";
    button.textContent = `@${agent.name}`;
    button.dataset.profileKind = "agent";
    button.dataset.profileId = agent.id;
    button.dataset.profileName = agent.name;
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

  const query = match.query.toLowerCase();
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
    <button type="button" class="mention-suggestion ${selected ? "active" : ""}" data-agent-id="${escapeHTML(agent.id)}" data-profile-kind="agent" data-profile-id="${escapeHTML(agent.id)}" data-profile-name="${escapeHTML(agent.name)}">
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
  const mention = `@${agent.name.replace(/\s+/g, "-")} `;
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

async function sendComposerMessage() {
  const text = els.input.value.trim();
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

els.composer.addEventListener("submit", async (event) => {
  event.preventDefault();
  await sendComposerMessage();
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

document.querySelector("#new-agent").addEventListener("click", openAgentDialog);
document.querySelector("#new-user").addEventListener("click", () => els.userDialog.showModal());
document.querySelector("#new-channel").addEventListener("click", () => els.channelDialog.showModal());
els.openSkills.addEventListener("click", () => {
  state.view = "skills";
  render();
});
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
els.markdownDialogClose.addEventListener("click", () => els.markdownDialog.close());

function openAgentDialog() {
  renderAgentSkillLibrary();
  els.agentDialog.showModal();
}

document.querySelector("#agent-create").addEventListener("click", async (event) => {
  event.preventDefault();
  const name = els.agentName.value.trim();
  const persona = els.agentPersona.value.trim();
  const systemPrompt = els.agentSystemPrompt.value.trim();
  const runtime = els.agentRuntime.value;
  const selectedModel = els.agentModel.value;
  const model = selectedModel === "__custom" ? els.agentModelCustom.value.trim() : selectedModel;
  const skills = parseInitialSkills(els.agentSkills.value);
  const skillIds = selectedAgentSkillIDs();
  if (!name) return;
  await api("/api/agents", { method: "POST", body: JSON.stringify({ name, persona, systemPrompt, runtime, model, skills, skillIds }) });
  els.agentName.value = "";
  els.agentPersona.value = "";
  els.agentSystemPrompt.value = "";
  els.agentRuntime.value = "codex";
  els.agentModelCustom.value = "";
  els.agentSkills.value = "";
  renderAgentSkillLibrary();
  populateModelOptions("codex");
  els.agentDialog.close();
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
els.skillAttach.addEventListener("click", async () => {
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
});

for (const button of els.skillModeButtons) {
  button.addEventListener("click", () => {
    state.skillCreateMode = button.dataset.skillMode === "cloud" ? "cloud" : "local";
    clearSkillError();
    updateSkillCreateModeUI();
  });
}

document.querySelector("#skill-import").addEventListener("click", async (event) => {
  event.preventDefault();
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
    setSkillError("Cloud import supports skills.sh links and GitHub links.");
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

async function deleteChannel(channel) {
  if (!window.confirm(`Delete #${channel.name}? This removes the channel and its messages.`)) return;
  try {
    await api(`/api/channels/${encodeURIComponent(channel.id)}`, { method: "DELETE" });
    if (state.channelId === channel.id) {
      const next = state.snapshot.channels.find((candidate) => candidate.id !== channel.id);
      state.channelId = next?.id || "";
    }
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
  state.skillCreateMode = "local";
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
  const isCloudImport = state.skillCreateMode === "cloud";
  for (const button of els.skillModeButtons) {
    const active = button.dataset.skillMode === state.skillCreateMode;
    button.classList.toggle("active", active);
    button.setAttribute("aria-pressed", String(active));
  }
  els.skillLocalFields.hidden = isCloudImport;
  els.skillNameLabel.textContent = isCloudImport ? "Skill name (optional)" : "Skill name";
  els.skillSourceLabel.textContent = isCloudImport ? "Cloud URL" : "Source note";
  els.skillSource.placeholder = isCloudImport
    ? "https://skills.sh/owner/repo/skill or https://github.com/owner/repo/blob/main/path/SKILL.md"
    : "SKILL.md or internal note";
  const action = state.skillDialogMode === "global"
    ? (isCloudImport ? "Import" : "Create")
    : (isCloudImport ? "Import & Attach" : "Create & Attach");
  document.querySelector("#skill-import").textContent = action;
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
  const compacted = cleanMarkdownInline(content.replace(/\s+/g, " "));
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
  els.markdownDialogBody.innerHTML = renderMarkdown(skill.content || "");
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
  els.skillAttachRow.hidden = false;
  updateSkillCreateModeUI();
  els.skillList.innerHTML = "";
  if (!agent) return;
  renderSkillAttachOptions(agent);
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
  els.skillAttach.disabled = available.length === 0;
  if (available.length === 0) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No unattached skills";
    els.skillAttachSelect.append(option);
  }
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
  els.input.value += `${suffix}@${name.replace(/\s+/g, "-")} `;
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
  try {
    const url = new URL(value.trim());
    if (url.protocol !== "http:" && url.protocol !== "https:") return false;
    return ["skills.sh", "www.skills.sh", "github.com", "raw.githubusercontent.com"].includes(url.hostname.toLowerCase());
  } catch {
    return false;
  }
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

function linkMentions(text) {
  return text.replace(/@([^\s@<>()\[\]{}.,，。:：;；!?！？]+)/g, (full, token) => {
    const participant = participantForMention(token);
    if (!participant) return `<strong>${full}</strong>`;
    return `<strong class="mention-profile profile-anchor" ${profileAnchorAttrs(participant.kind, participant.value)} tabindex="0">${full}</strong>`;
  });
}

function participantForMention(token = "") {
  if (!state.snapshot) return null;
  const normalized = mentionSlug(token);
  const agent = (state.snapshot.agents || []).find((candidate) => mentionSlug(candidate.name) === normalized || mentionSlug(candidate.id) === normalized);
  if (agent) return { kind: "agent", value: agent };
  const user = (state.snapshot.users || []).find((candidate) => mentionSlug(candidate.name) === normalized || mentionSlug(candidate.id) === normalized);
  if (user) return { kind: "human", value: user };
  return null;
}

function mentionSlug(value = "") {
  return value.replace(/^@/, "").replace(/\s+/g, "-").toLowerCase();
}

function profileAnchorAttrs(kind, value) {
  return `data-profile-kind="${escapeHTML(kind)}" data-profile-id="${escapeHTML(value.id || "")}" data-profile-name="${escapeHTML(value.name || "")}"`;
}

function renderMarkdown(text = "") {
  const lines = text.split("\n");
  const html = [];
  let inCode = false;
  let codeLines = [];
  let listType = "";

  const closeList = () => {
    if (listType) {
      html.push(`</${listType}>`);
      listType = "";
    }
  };
  const openList = (type) => {
    if (listType === type) return;
    closeList();
    html.push(`<${type}>`);
    listType = type;
  };
  const closeCode = () => {
    html.push(`<pre><code>${escapeHTML(codeLines.join("\n"))}</code></pre>`);
    codeLines = [];
    inCode = false;
  };

  for (const line of lines) {
    if (line.trim().startsWith("```")) {
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
      closeList();
      continue;
    }

    const heading = trimmed.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      closeList();
      const level = Math.min(heading[1].length + 1, 4);
      html.push(`<h${level}>${inlineMarkdown(heading[2])}</h${level}>`);
      continue;
    }

    const bullet = trimmed.match(/^[-*]\s+(.+)$/);
    if (bullet) {
      openList("ul");
      html.push(`<li>${inlineMarkdown(bullet[1])}</li>`);
      continue;
    }

    const ordered = trimmed.match(/^\d+\.\s+(.+)$/);
    if (ordered) {
      openList("ol");
      html.push(`<li>${inlineMarkdown(ordered[1])}</li>`);
      continue;
    }

    if (trimmed.startsWith(">")) {
      closeList();
      html.push(`<blockquote>${inlineMarkdown(trimmed.replace(/^>\s?/, ""))}</blockquote>`);
      continue;
    }

    if (/^\|.+\|$/.test(trimmed)) {
      closeList();
      html.push(`<pre class="markdown-table">${escapeHTML(trimmed)}</pre>`);
      continue;
    }

    closeList();
    html.push(`<p>${inlineMarkdown(trimmed)}</p>`);
  }

  closeList();
  if (inCode) closeCode();
  return html.join("");
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

populateModelOptions("codex");
