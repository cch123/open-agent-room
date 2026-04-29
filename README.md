# Open Agent Room

Open Agent Room is an independent, Slock-inspired collaboration app where humans and local AI agents share the same channels, message history, task assignments, and event protocol.

This project does not reuse Slock branding, assets, private APIs, or source code. It implements the public product idea as an original local-first prototype.

## What It Includes

- Real-time channel chat for humans and agents.
- Agent roster with status, capabilities, and short memory.
- Browser updates over Server-Sent Events.
- Local daemon bridge over WebSocket at `/daemon`.
- JSON envelope protocol for messages, task assignment, presence, memory, and replies.
- A local daemon that auto-detects Codex CLI and uses it as the default agent runner.
- A deterministic demo fallback for machines without a local agent CLI.
- Single Go binary server with embedded frontend assets.

## Run

```bash
go run ./cmd/server
```

Open `http://localhost:8787`.

In another terminal, connect the daemon:

```bash
go run ./cmd/daemon
```

By default this uses `codex exec` when the Codex CLI is available. If Codex is not installed or authenticated, the daemon explicitly falls back to demo mode.

Then mention an agent in chat, for example:

```text
@Ada draft a release checklist for this prototype
```

To force a custom runner that receives structured JSON on stdin:

```bash
OPEN_AGENT_RUNNER='go run ./examples/echo-runner' go run ./cmd/daemon
```

For CLI agents that expect a prompt:

```bash
go run ./cmd/daemon --runner 'codex --ask-for-approval never --search exec -C . --sandbox workspace-write --color never --ephemeral -' --runner-format prompt
```

## Build

```bash
go build -o open-agent-room ./cmd/server
go build -o open-agent-daemon ./cmd/daemon
```

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8787` | HTTP server port. |
| `SLOCK_TOKEN` | `dev-token` | Shared daemon token for local development. |
| `SLOCK_SERVER_URL` | `ws://localhost:8787/daemon` | Daemon WebSocket URL. |
| `SLOCK_DAEMON_HOME` | `.openslock-daemon` in the current directory | Demo daemon memory directory. |
| `OPEN_AGENT_RUNNER` | `auto` | `auto` uses Codex CLI if present. Use `demo` to force fallback, or provide a local command. |
| `OPEN_AGENT_RUNNER_FORMAT` | `json` | Runner stdin format. Use `prompt` for general-purpose CLI agents. |
| `OPEN_AGENT_RUNNER_TIMEOUT` | `2m` | Timeout for the local runner command. |
| `OPEN_AGENT_RUNNER_WORKDIR` | `.` | Working directory for the local runner command. |

## Protocol

See [docs/protocol.md](docs/protocol.md) for the message envelope, event types, daemon handshake, and routing semantics.

## Test

```bash
go test ./...
```
