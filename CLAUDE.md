# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

cc-sdk-go is a Go SDK that turns Claude Code CLI into an OpenAI-compatible inference proxy. The `cc-proxy` binary accepts OpenAI chat completion requests, translates them into Claude Code subprocess calls, and streams back OpenAI-format responses. Each request spawns an isolated `claude` process (stateless).

## Commands

```bash
# Build the proxy binary
go build ./cmd/cc-proxy

# Run the proxy
go run ./cmd/cc-proxy -addr=:8080 -model=sonnet

# Run all tests (requires claude CLI authenticated)
go test ./...

# Run a single test
go test ./oai -run TestCompletion

# Vet
go vet ./...
```

Tests in `oai/client_test.go` are integration tests that spawn real `claude` processes — they require the Claude Code CLI installed and authenticated.

## Architecture

```
cmd/cc-proxy  →  server  →  oai (bridge)  →  cchat (subprocess)  →  ccwire (parser)
```

**Dependency flow is strictly left-to-right, bottom-up:**

- **ccwire** — Wire format types (`SystemMessage`, `AssistantMessage`, `ResultMessage`, `StreamEventMessage`) and NDJSON parser for Claude Code CLI stdout
- **cchat** — Wraps the `claude` CLI as a subprocess. `Client` manages concurrency (buffered channel semaphore). `Stream` reads parsed messages from the process. Prompt is piped via stdin to avoid OS arg length limits
- **oai** — OpenAI-compatible types (request/response/chunk) plus the bridge layer:
  - `bridge_request.go`: OAI messages → CC prompt string (roles as `[user]:` prefixed text, tool results as `[tool_result for call_id]:`)
  - `bridge_response.go`: CC result → OAI response (extracts text, parses tool calls, maps usage)
  - `bridge_stream.go`: Streaming state machine with safety margin — holds back last `len("<tool_call>")` bytes to prevent partial XML tag leaks
  - `tools.go`: `ParseToolCalls()` extracts `<tool_call>` XML tags via regex, `ToolCallInstructions()` generates system prompt text for tool use
  - `client.go` / `client_stream.go`: OpenAI-shaped client wrapping `cchat.Client`
- **server** — HTTP server with `POST /v1/chat/completions` and `GET /v1/models`. Middleware stack: panic recovery → logging → auth (optional Bearer token)
- **cmd/cc-proxy** — CLI entry point, flag parsing

## Key Design Decisions

- **Tool calls via system prompt injection**: Tool definitions are converted to text instructions telling the model to output `<tool_call>` XML tags, which are then parsed back into structured OpenAI `tool_calls`. This is not native tool use — it's prompt-engineered.
- **Full system prompt replacement**: `--system-prompt` is always specified alongside `--tools=""` (disabling CC built-in tools) for deterministic prompts and better cache hit rates.
- **Streaming safety margin**: When tools are enabled, the stream buffer withholds the last N bytes to avoid emitting partial `<tool_call>` tags as text content. On stream finish, full text is parsed authoritatively.
- **Zero dependencies**: Only stdlib — no external Go modules.
