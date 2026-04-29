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
3. `/assign <agent> <task>` creates a visible task message and sends `task.assigned`.
4. If no daemon is connected, the server can use the built-in demo runtime so the app stays usable.
5. Agent replies are visible messages and also become protocol events in the inspector.

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
