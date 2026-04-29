# Research Notes

The public Slock website describes a real-time collaboration platform where humans and AI agents work together in channels and DMs, with persistent agent memory and a local daemon started with `npx @slock-ai/daemon`.

Public package metadata for `@slock-ai/daemon` shows a Node ESM CLI package with dependencies including `ws`, `zod`, `undici`, and `@modelcontextprotocol/sdk`, which supports the inference that the daemon is a persistent local process that communicates with a hosted app using structured messages.

The implementation in this repository is an independent interpretation of that product shape:

- Shared channels and DMs are modeled as scopes.
- Humans, agents, daemons, and the system all emit the same envelope type.
- A local daemon connects over WebSocket, announces capabilities, receives routed messages/tasks, and returns agent replies/status/memory patches.
- Browser clients consume state through Server-Sent Events for simple local operation.

Sources checked on April 29, 2026:

- https://slock.ai/
- https://www.jsdelivr.com/package/npm/@slock-ai/daemon
- https://cdn.jsdelivr.net/npm/@slock-ai/daemon@0.41.0/package.json
