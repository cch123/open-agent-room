const state = {
  snapshot: null,
  channelId: "chan_general",
  selectedEventId: null,
};

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
  agentList: document.querySelector("#agent-list"),
  roomName: document.querySelector("#room-name"),
  daemonChip: document.querySelector("#daemon-chip"),
  daemonCount: document.querySelector("#daemon-count"),
  messages: document.querySelector("#messages"),
  composer: document.querySelector("#composer"),
  input: document.querySelector("#message-input"),
  mentionRow: document.querySelector("#mention-row"),
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
  agentRuntime: document.querySelector("#agent-runtime"),
  agentModel: document.querySelector("#agent-model"),
  agentModelCustom: document.querySelector("#agent-model-custom"),
  agentModelCustomRow: document.querySelector("#agent-model-custom-row"),
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
  const { channels, agents, daemons, events } = state.snapshot;
  if (!channels.some((channel) => channel.id === state.channelId)) {
    state.channelId = channels[0]?.id || "";
  }
  const current = channels.find((channel) => channel.id === state.channelId);
  els.roomName.textContent = current ? `#${current.name}` : "#channel";

  renderChannels(channels);
  renderAgents(agents);
  renderMessages();
  renderMentions(agents);
  renderDaemon(daemons);
  renderChannelSettings(current, agents);
  renderAssign(agents);
  renderEvents(events || []);
}

function renderChannels(channels) {
  els.channelList.innerHTML = "";
  for (const channel of channels) {
    const row = document.createElement("div");
    row.className = "nav-row";
    const button = document.createElement("button");
    button.className = `nav-item ${channel.id === state.channelId ? "active" : ""}`;
    button.title = channel.topic ? `#${channel.name} - ${channel.topic}` : `#${channel.name}`;
    button.innerHTML = `<span class="hash">#</span><span><strong>${escapeHTML(channel.name)}</strong><span class="nav-topic">${escapeHTML(channel.topic || "")}</span></span>`;
    button.addEventListener("click", () => {
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

function renderAgents(agents) {
  els.agentList.innerHTML = "";
  for (const agent of agents) {
    const row = document.createElement("div");
    row.className = "agent-row";
    const button = document.createElement("button");
    button.className = "agent-item";
    const meta = `${agent.status} · ${runtimeLabel(agent)} · ${agent.persona}`;
    button.title = `${agent.name} - ${meta}`;
    button.innerHTML = `<span class="avatar" style="background:${agent.color || "#2563eb"}">${initials(agent.name)}</span><span><strong>${escapeHTML(agent.name)}</strong><span class="agent-meta">${escapeHTML(meta)}</span></span>`;
    button.addEventListener("click", () => {
      insertMention(agent.name);
    });
    const deleteButton = document.createElement("button");
    deleteButton.type = "button";
    deleteButton.className = "item-delete";
    deleteButton.title = `Delete ${agent.name}`;
    deleteButton.setAttribute("aria-label", `Delete agent ${agent.name}`);
    deleteButton.textContent = "x";
    deleteButton.addEventListener("click", () => deleteAgent(agent));
    row.append(button, deleteButton);
    els.agentList.append(row);
  }
}

function renderMessages() {
  const { messages, agents } = state.snapshot;
  const byAgent = new Map(agents.map((agent) => [agent.id, agent]));
  const visible = messages.filter((message) => message.channelId === state.channelId);
  els.messages.innerHTML = "";
  for (const message of visible) {
    const item = document.createElement("article");
    item.className = `message ${message.kind || ""} ${message.authorKind || ""}`;
    const agent = byAgent.get(message.authorId);
    const color = agent?.color || (message.authorKind === "human" ? "#2563eb" : "#64748b");
    const content = renderMessageContent(message);
    item.innerHTML = `
      <span class="avatar" style="background:${color}">${initials(message.authorName)}</span>
      <div>
        <div class="message-header">
          <span class="message-name">${escapeHTML(message.authorName)}</span>
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

function markdownDocumentParts(message) {
  const text = message.text || "";
  if (!isMarkdownDocumentMessage(message)) {
    return { before: text, document: "", after: "" };
  }
  const lines = text.split("\n");
  const start = markdownDocumentStartLine(lines);
  if (start <= 0) {
    return { before: "", document: text.trim(), after: "" };
  }
  return {
    before: lines.slice(0, start).join("\n").trim(),
    document: lines.slice(start).join("\n").trim(),
    after: "",
  };
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
    button.addEventListener("click", () => insertMention(agent.name));
    els.mentionRow.append(button);
  }
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

els.composer.addEventListener("submit", async (event) => {
  event.preventDefault();
  const text = els.input.value.trim();
  if (!text) return;
  els.input.value = "";
  try {
    await api("/api/messages", {
      method: "POST",
      body: JSON.stringify({ channelId: state.channelId, text }),
    });
  } catch (error) {
    alert(error.message);
  }
});

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

document.querySelector("#new-agent").addEventListener("click", () => els.agentDialog.showModal());
document.querySelector("#new-channel").addEventListener("click", () => els.channelDialog.showModal());

els.agentRuntime.addEventListener("change", () => populateModelOptions(els.agentRuntime.value));
els.agentModel.addEventListener("change", updateCustomModelVisibility);
els.markdownDialogClose.addEventListener("click", () => els.markdownDialog.close());

document.querySelector("#agent-create").addEventListener("click", async (event) => {
  event.preventDefault();
  const name = els.agentName.value.trim();
  const persona = els.agentPersona.value.trim();
  const runtime = els.agentRuntime.value;
  const selectedModel = els.agentModel.value;
  const model = selectedModel === "__custom" ? els.agentModelCustom.value.trim() : selectedModel;
  if (!name) return;
  await api("/api/agents", { method: "POST", body: JSON.stringify({ name, persona, runtime, model }) });
  els.agentName.value = "";
  els.agentPersona.value = "";
  els.agentRuntime.value = "codex";
  els.agentModelCustom.value = "";
  populateModelOptions("codex");
  els.agentDialog.close();
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

function insertMention(name) {
  const suffix = els.input.value && !els.input.value.endsWith(" ") ? " " : "";
  els.input.value += `${suffix}@${name.replace(/\s+/g, "-")} `;
  els.input.focus();
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
  return text.replace(/@([a-z0-9][a-z0-9_-]{0,40})/gi, "<strong>@$1</strong>");
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
