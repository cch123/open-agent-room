SHELL := /bin/sh

PORT ?= 8787
DATA_PATH ?= data/state.json
SLOCK_TOKEN ?= dev-token
SLOCK_SERVER_URL ?= ws://localhost:$(PORT)/daemon
OPEN_AGENT_RUNNER ?= auto
OPEN_AGENT_RUNNER_FORMAT ?= json
OPEN_AGENT_RUNNER_TIMEOUT ?= 2m
OPEN_AGENT_RUNNER_WORKDIR ?= .
OPEN_AGENT_ARBITER_RUNTIME ?= codex
OPEN_AGENT_ARBITER_MODEL ?=

.PHONY: help dev server daemon demo-daemon test build clean

help:
	@printf "Open Agent Room targets:\n"
	@printf "  make dev          start server and local daemon together\n"
	@printf "  make server       start only the web server on PORT=%s\n" "$(PORT)"
	@printf "  make daemon       connect only the local daemon\n"
	@printf "  make demo-daemon  connect daemon in deterministic demo mode\n"
	@printf "  make test         run Go tests\n"
	@printf "  make build        build server and daemon binaries\n"
	@printf "\nConfig: PORT DATA_PATH SLOCK_TOKEN SLOCK_SERVER_URL OPEN_AGENT_RUNNER OPEN_AGENT_RUNNER_FORMAT OPEN_AGENT_RUNNER_TIMEOUT OPEN_AGENT_RUNNER_WORKDIR OPEN_AGENT_ARBITER_RUNTIME OPEN_AGENT_ARBITER_MODEL\n"

dev:
	@set -eu; \
	echo "Starting Open Agent Room at http://localhost:$(PORT)"; \
	server_pid=""; \
	daemon_pid=""; \
	cleanup() { \
		if [ -n "$$server_pid" ]; then kill $$server_pid 2>/dev/null || true; fi; \
		if [ -n "$$daemon_pid" ]; then kill $$daemon_pid 2>/dev/null || true; fi; \
	}; \
	trap cleanup INT TERM EXIT; \
	PORT="$(PORT)" DATA_PATH="$(DATA_PATH)" SLOCK_TOKEN="$(SLOCK_TOKEN)" go run ./cmd/server & \
	server_pid=$$!; \
	sleep 1; \
	SLOCK_SERVER_URL="$(SLOCK_SERVER_URL)" SLOCK_TOKEN="$(SLOCK_TOKEN)" OPEN_AGENT_RUNNER="$(OPEN_AGENT_RUNNER)" OPEN_AGENT_RUNNER_FORMAT="$(OPEN_AGENT_RUNNER_FORMAT)" OPEN_AGENT_RUNNER_TIMEOUT="$(OPEN_AGENT_RUNNER_TIMEOUT)" OPEN_AGENT_RUNNER_WORKDIR="$(OPEN_AGENT_RUNNER_WORKDIR)" OPEN_AGENT_ARBITER_RUNTIME="$(OPEN_AGENT_ARBITER_RUNTIME)" OPEN_AGENT_ARBITER_MODEL="$(OPEN_AGENT_ARBITER_MODEL)" go run ./cmd/daemon & \
	daemon_pid=$$!; \
	echo "Server PID $$server_pid, daemon PID $$daemon_pid. Press Ctrl-C to stop both."; \
	while kill -0 $$server_pid 2>/dev/null && kill -0 $$daemon_pid 2>/dev/null; do \
		sleep 1; \
	done

server:
	PORT="$(PORT)" DATA_PATH="$(DATA_PATH)" SLOCK_TOKEN="$(SLOCK_TOKEN)" go run ./cmd/server

daemon:
	SLOCK_SERVER_URL="$(SLOCK_SERVER_URL)" SLOCK_TOKEN="$(SLOCK_TOKEN)" OPEN_AGENT_RUNNER="$(OPEN_AGENT_RUNNER)" OPEN_AGENT_RUNNER_FORMAT="$(OPEN_AGENT_RUNNER_FORMAT)" OPEN_AGENT_RUNNER_TIMEOUT="$(OPEN_AGENT_RUNNER_TIMEOUT)" OPEN_AGENT_RUNNER_WORKDIR="$(OPEN_AGENT_RUNNER_WORKDIR)" OPEN_AGENT_ARBITER_RUNTIME="$(OPEN_AGENT_ARBITER_RUNTIME)" OPEN_AGENT_ARBITER_MODEL="$(OPEN_AGENT_ARBITER_MODEL)" go run ./cmd/daemon

demo-daemon:
	$(MAKE) daemon OPEN_AGENT_RUNNER=demo

test:
	go test -count=1 ./...

build:
	go build -o open-agent-room ./cmd/server
	go build -o open-agent-daemon ./cmd/daemon

clean:
	rm -f open-agent-room open-agent-daemon
