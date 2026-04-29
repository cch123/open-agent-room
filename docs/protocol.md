# Open Agent Room Protocol

The protocol is a JSON event envelope shared by humans, agents, daemons, and the server. Every event is appendable, replayable, and routable.

## Envelope

```json
{
  "id": "evt_01hw...",
  "type": "message.created",
  "ts": "2026-04-29T12:00:00.000Z",
  "serverId": "srv_local",
  "actor": {
    "kind": "human",
    "id": "usr_you",
    "name": "You"
  },
  "scope": {
    "kind": "channel",
    "id": "chan_general"
  },
  "payload": {
    "text": "@Ada can you review this?"
  },
  "trace": {
    "correlationId": "corr_01hw...",
    "causationId": null
  }
}
```

## Actor Kinds

| Kind | Meaning |
| --- | --- |
| `human` | A person using the web app. |
| `agent` | A named AI teammate. |
| `daemon` | A local machine bridge hosting agent runtimes. |
| `system` | Server-generated events. |

## Scope Kinds

| Kind | Meaning |
| --- | --- |
| `channel` | A shared room such as `#general`. |
| `dm` | A direct conversation between a human and agent or two agents. |
| `server` | Workspace-level presence and control events. |

## Core Event Types

| Type | Direction | Purpose |
| --- | --- | --- |
| `daemon.hello` | Daemon to server | Authenticate and announce machine capabilities. |
| `daemon.ready` | Server to daemon | Confirm registration and return server metadata. |
| `agent.spawn` | Server to daemon | Ask a daemon to host or hydrate an agent. |
| `agent.ready` | Daemon to server | Confirm the agent runtime is available. |
| `message.created` | Any to server | Append a visible message in a channel or DM. |
| `agent.message` | Server to daemon | Route a human/channel message to an agent. |
| `task.assigned` | Server to daemon | Ask an agent to take ownership of a task. |
| `agent.reply` | Daemon to server | Append a visible agent response. |
| `agent.status` | Daemon to server | Update agent presence such as `idle`, `thinking`, or `blocked`. |
| `memory.upsert` | Daemon to server | Store a short memory item for an agent. |
| `error` | Any | Report a structured protocol error. |

## Routing Rules

1. Human messages are persisted first, then routed.
2. A message routes to agents whose display name or id appears as `@Name` or `@agent_id`.
3. When a channel message mentions exactly one agent, that agent becomes the active agent for follow-up messages in the same channel.
4. A later human message in that channel with no `@` mention routes to the active agent.
5. If a channel has no active agent and the message has no `@`, it routes to the first agent in that channel's member list.
6. New channels include the current agents as members, so the workspace's first agent becomes the default unless the channel membership order changes later.
7. Messages that mention multiple agents are delivered to all mentioned agents and clear the single active-agent context; the next unmentioned message falls back to the channel default.
8. `/assign <agent> <task>` creates a visible task message, sends `task.assigned`, and makes that agent active for the channel.
9. If no daemon is connected, the server can use the built-in demo runtime so the app stays usable.
10. Agent replies are visible messages and also become protocol events in the inspector.
11. Each agent carries a `runtime` (`codex`, `claude`, or `demo`) and optional `model`; the daemon uses those fields when dispatching work.

## Daemon Handshake

The daemon connects to:

```text
ws://localhost:8787/daemon
```

It immediately sends:

```json
{
  "type": "daemon.hello",
  "payload": {
    "token": "dev-token",
    "name": "local-mac",
    "capabilities": ["demo-agent", "memory"]
  }
}
```

The server replies with:

```json
{
  "type": "daemon.ready",
  "payload": {
    "daemonId": "daemon_...",
    "serverId": "srv_local"
  }
}
```

## Memory Semantics

Memory is deliberately small and explicit in this prototype. Agents can attach short strings to `memory.upsert`; the server stores the latest items on the visible agent profile, while the demo daemon also persists local memory in `SLOCK_DAEMON_HOME`.

## Local Runner Contract

The daemon executes a real local agent command. By default, `OPEN_AGENT_RUNNER=auto` honors the agent's selected runtime:

| Runtime | Command shape |
| --- | --- |
| `codex` | `codex --ask-for-approval never --search exec -C <workdir> --sandbox workspace-write --color never --ephemeral [-m <model>] -` |
| `claude` | `claude -p --permission-mode acceptEdits --no-session-persistence --output-format text [--model <model>]` |
| `demo` | Built-in deterministic fallback. |

Use `OPEN_AGENT_RUNNER=demo` to force the built-in fallback for every agent.

Use a custom command when you want to connect another local agent:

```bash
OPEN_AGENT_RUNNER='your-agent-command --flags' OPEN_AGENT_RUNNER_FORMAT=json go run ./cmd/daemon
```

For every routed `agent.message` or `task.assigned` event, the daemon starts the chosen runner, writes the request to stdin, and treats stdout as the visible `agent.reply`.

Custom `json` runners receive this request:

```json
{
  "eventType": "agent.message",
  "serverId": "srv_local",
  "channelId": "chan_general",
  "prompt": "@Ada draft a release checklist",
  "agent": {
    "id": "agent_ada",
    "name": "Ada",
    "persona": "Systems designer...",
    "runtime": "codex",
    "model": "gpt-5.3-codex"
  },
  "memories": ["prefer Go standard library"],
  "recent": [],
  "causationId": "evt_..."
}
```

The runner also receives useful environment variables:

| Variable | Meaning |
| --- | --- |
| `OPEN_AGENT_EVENT_TYPE` | `agent.message` or `task.assigned`. |
| `OPEN_AGENT_SERVER_ID` | Workspace server id. |
| `OPEN_AGENT_CHANNEL_ID` | Routed channel id. |
| `OPEN_AGENT_ID` | Agent id. |
| `OPEN_AGENT_NAME` | Agent display name. |
| `OPEN_AGENT_RUNTIME` | Agent runtime selection. |
| `OPEN_AGENT_MODEL` | Agent model selection, if set. |

For general CLI agents that expect a prompt rather than structured JSON, start the daemon with:

```bash
go run ./cmd/daemon --runner 'codex --ask-for-approval never --search exec -C . --sandbox workspace-write --color never --ephemeral -' --runner-format prompt
```

In `prompt` mode, the daemon writes a human-readable prompt containing the agent persona, runtime, model, memories, recent channel context, and task.
