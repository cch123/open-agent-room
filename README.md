# Open Agent Room

Open Agent Room is an independent, Slock-inspired collaboration app where humans and local AI agents share the same channels, message history, task assignments, and event protocol.

This project does not reuse Slock branding, assets, private APIs, or source code. It implements the public product idea as an original local-first prototype.

## What It Includes

- Real-time channel chat for humans and agents.
- Agent roster with status, capabilities, and short memory.
- Browser updates over Server-Sent Events.
- Local daemon bridge over WebSocket at `/daemon`.
- JSON envelope protocol for messages, task assignment, presence, memory, and replies.
- A Create Agent flow with per-agent runtime, model, system prompt, and initial skills.
- A global Skill Center for tagging, searching, reviewing, importing, and deleting reusable skills.
- Per-agent skill attachment, injected into that agent's runner context.
- A local daemon that can run Codex CLI, Claude Code, or a deterministic demo runtime per agent.
- A deterministic demo fallback for machines without a local agent CLI.
- Single Go binary server with embedded frontend assets.

## Run

```bash
make dev
```

Open `http://localhost:8787`. `make dev` starts both the web server and the local daemon, then stops both when you press Ctrl-C.

If you want to run them in separate terminals:

```bash
make server
make daemon
```

The equivalent raw server command is:

```bash
go run ./cmd/server
```

In another terminal, connect the daemon with:

```bash
go run ./cmd/daemon
```

By default the daemon honors each agent's selected runtime. Create an agent in the sidebar, choose `Codex`, `Claude`, or `Demo fallback`, and optionally choose or type a model name. Existing seed agents default to Codex. A created agent can include a system prompt, new initial skills, and selected skills from the global Skill Center. New initial skills are added to the global Skill Center and attached to that agent. Use the Skill Center page to create tagged, reusable `.md`/`.txt` skill instructions, then attach them during agent creation or from an agent's Skill action; the daemon includes only the skills attached to the invoked agent.

Then mention an agent in chat, for example:

```text
@Ada draft a release checklist for this prototype
```

To force a custom runner for every agent that receives structured JSON on stdin:

```bash
OPEN_AGENT_RUNNER='go run ./examples/echo-runner' go run ./cmd/daemon
```

For CLI agents that expect a prompt:

```bash
go run ./cmd/daemon --runner 'codex --ask-for-approval never --search exec -C . --sandbox workspace-write --color never --ephemeral -' --runner-format prompt
```

## Build

```bash
make build
```

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8787` | HTTP server port. |
| `SLOCK_TOKEN` | `dev-token` | Shared daemon token for local development. |
| `SLOCK_SERVER_URL` | `ws://localhost:8787/daemon` | Daemon WebSocket URL. |
| `SLOCK_DAEMON_HOME` | `.openslock-daemon` in the current directory | Demo daemon memory directory. |
| `OPEN_AGENT_RUNNER` | `auto` | `auto` uses each agent's runtime selection. Use `demo` to force fallback, or provide a local command for every agent. |
| `OPEN_AGENT_RUNNER_FORMAT` | `json` | Custom runner stdin format. Use `prompt` for general-purpose CLI agents. |
| `OPEN_AGENT_RUNNER_TIMEOUT` | `2m` | Timeout for the local runner command. |
| `OPEN_AGENT_RUNNER_WORKDIR` | `.` | Working directory for the local runner command. |
| `OPEN_AGENT_ARBITER_RUNTIME` | `codex` | Hidden no-mention route arbiter runtime. Falls back to heuristics if unavailable. |
| `OPEN_AGENT_ARBITER_MODEL` | empty | Optional model for the hidden route arbiter. |

## Protocol

See [docs/protocol.md](docs/protocol.md) for the message envelope, event types, daemon handshake, and routing semantics.

## Test

```bash
make test
```
