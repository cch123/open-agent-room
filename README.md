# Open Agent Room

Open Agent Room is an independent, Slock-inspired collaboration app where humans and local AI agents share the same channels, message history, task assignments, and event protocol.

This project does not reuse Slock branding, assets, private APIs, or source code. It implements the public product idea as an original local-first prototype.

## What It Includes

- Real-time channel chat for humans and agents.
- Agent roster with status, capabilities, and short memory.
- Browser updates over Server-Sent Events.
- Local daemon bridge over WebSocket at `/daemon`.
- JSON envelope protocol for messages, task assignment, presence, memory, and replies.
- A deterministic demo daemon that can run without cloud AI keys.
- Single Go binary server with embedded frontend assets.

## Run

```bash
go run ./cmd/server
```

Open `http://localhost:8787`.

In another terminal, connect the demo daemon:

```bash
go run ./cmd/daemon
```

Then mention an agent in chat, for example:

```text
@Ada draft a release checklist for this prototype
```

By default, the daemon uses a deterministic demo runtime so the protocol works without API keys. To make a real local agent handle tasks, provide a runner command:

```bash
OPEN_AGENT_RUNNER='go run ./examples/echo-runner' go run ./cmd/daemon
```

The runner receives a JSON request on stdin and writes the visible agent reply to stdout. You can replace the example with a CLI agent:

```bash
go run ./cmd/daemon --runner 'codex exec -C . -' --runner-format prompt
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
| `OPEN_AGENT_RUNNER` | empty | Optional local command that handles each routed agent task. |
| `OPEN_AGENT_RUNNER_FORMAT` | `json` | Runner stdin format. Use `prompt` for general-purpose CLI agents. |
| `OPEN_AGENT_RUNNER_TIMEOUT` | `2m` | Timeout for the local runner command. |
| `OPEN_AGENT_RUNNER_WORKDIR` | `.` | Working directory for the local runner command. |

## Protocol

See [docs/protocol.md](docs/protocol.md) for the message envelope, event types, daemon handshake, and routing semantics.

## Test

```bash
go test ./...
```
