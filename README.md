# cc-sdk-go

**Turn your Claude Code subscription into an OpenAI-compatible API.** One binary. Zero config. Pure stdlib.

```bash
go install github.com/codewandler/cc-sdk-go/cmd/cc-proxy@latest
cc-proxy
# That's it. localhost:8080 now speaks OpenAI.
```

Any tool, framework, or script that talks to the OpenAI chat completions API now works with Claude — Cursor, Continue, aider, open-code, LangChain, your weekend hack, all of it.

You handle the agentic loop. Claude handles the thinking.

---

## Why does this exist?

Anthropic killed API token extraction from the Claude Code CLI. Every tool that relied on it broke overnight.

`cc-sdk-go` takes a different approach: spawn the official `claude` CLI as a subprocess, pipe in your prompt, stream back the response — all behind a standard `/v1/chat/completions` endpoint.

No token extraction. No reverse engineering. No ToS violations. Just the official CLI doing what it was built to do.

| | Token Extraction (Dead) | cc-sdk-go |
|---|---|---|
| **Method** | Reverse-engineering internal storage | Official CLI subprocesses |
| **Status** | Broken | Works |
| **ToS** | Violates Anthropic Terms | Fully compliant |
| **Setup** | Complex extraction scripts | `go install` + `cc-proxy` |

**Trade-off**: You lose fine-grained sampling parameters (temperature, top_p, etc.) but gain a setup that won't break next week.

---

## How it works

```
POST /v1/chat/completions
        |
        v
  translate OAI format --> CC prompt
        |
        v
  claude --print (subprocess, stdin pipe)
        |
        v
  NDJSON stdout --> parse --> SSE stream back
```

Each request is stateless. Spawn process, pipe prompt, stream response, exit. No session state, no connection pools, no daemon. Claude's built-in tools are disabled — your framework provides the tools, cc-proxy just does inference.

---

## Quick start

**Prerequisites**: Go 1.25+, [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) authenticated (`claude setup-token`)

```bash
# Install
go install github.com/codewandler/cc-sdk-go/cmd/cc-proxy@latest

# Run
cc-proxy -addr=:8080 -model=sonnet

# Test it
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"sonnet","messages":[{"role":"user","content":"Hello!"}],"stream":true}'
```

Point any OpenAI client at `http://localhost:8080/v1` and go.

---

## Use it with anything

**Python (OpenAI SDK)**
```python
from openai import OpenAI

client = OpenAI(base_url="http://localhost:8080/v1", api_key="any")
resp = client.chat.completions.create(
    model="sonnet",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(resp.choices[0].message.content)
```

**Go (go-openai)**
```go
cfg := openai.DefaultConfig("any")
cfg.BaseURL = "http://localhost:8080/v1"
client := openai.NewClientWithConfig(cfg)

resp, _ := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
    Model:    "sonnet",
    Messages: []openai.ChatCompletionMessage{
        {Role: "user", Content: "Hello!"},
    },
})
```

**Cursor / Continue / aider / open-code / anything**
```
Base URL:  http://localhost:8080/v1
API Key:   anything
Model:     sonnet
```

---

## Tool calls

Send OpenAI-format tool definitions. The proxy injects them into the system prompt, the model outputs `<tool_call>` XML, and cc-proxy parses it back into structured `tool_calls`. Your framework never sees the XML.

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sonnet",
    "messages": [{"role": "user", "content": "What is the weather in NYC?"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get current weather",
        "parameters": {
          "type": "object",
          "properties": {"location": {"type": "string"}},
          "required": ["location"]
        }
      }
    }]
  }'
```

Response (standard OpenAI format):
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "tool_calls": [{
        "id": "call_0",
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\":\"NYC\"}"
        }
      }],
      "content": "I'll check the weather in NYC for you."
    },
    "finish_reason": "tool_calls"
  }]
}
```

---

## CLI flags

```
cc-proxy [flags]

  -addr string          Listen address (default ":8080")
  -model string         Default model (sonnet, opus, haiku)
  -api-key string       API key for Bearer auth (empty = no auth)
  -claude-path string   Path to claude binary (default "claude")
  -max-concurrent int   Max concurrent claude processes (0 = unlimited)
  -timeout duration     Per-request timeout (default 5m)
  -work-dir string      Working directory for claude processes
```

API key can also be set via `CC_PROXY_API_KEY` env var.

**Endpoints**: `POST /v1/chat/completions` (streaming + non-streaming), `GET /v1/models`

---

## Use as a Go library

Skip the HTTP server entirely and use the SDK directly.

### Low-level: `cchat.Client`

Direct subprocess control. You get raw Claude Code wire messages.

```go
import "github.com/codewandler/cc-sdk-go/cchat"

client := cchat.NewClient(&cchat.ClientConfig{
    Model:         "sonnet",
    MaxConcurrent: 4,
})

stream, err := client.Query(ctx, "What is 2+2?", cchat.QueryOptions{
    SystemPrompt: "You are a helpful math tutor.",
})
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

result, err := stream.Result()
fmt.Println(result.Result) // "4"
```

Streaming:
```go
stream, _ := client.Query(ctx, "Explain gravity", cchat.QueryOptions{
    Streaming: true,
})
defer stream.Close()

for {
    msg, err := stream.Next()
    if err == io.EOF {
        break
    }
    // msg is *ccwire.StreamEventMessage, *ccwire.AssistantMessage, etc.
}
```

### High-level: `oai.Client`

OpenAI-shaped request/response types without the HTTP server.

```go
import "github.com/codewandler/cc-sdk-go/oai"

client := oai.NewClientDefault()

resp, err := client.CreateChatCompletion(ctx, oai.ChatCompletionRequest{
    Model: "sonnet",
    Messages: []oai.ChatMessage{
        {Role: "system", Content: "You are a helpful assistant."},
        {Role: "user", Content: "What is 2+2?"},
    },
})
fmt.Println(resp.Choices[0].Message.StringContent())
```

Streaming:
```go
stream, _ := client.CreateChatCompletionStream(ctx, oai.ChatCompletionRequest{
    Model:    "sonnet",
    Messages: []oai.ChatMessage{{Role: "user", Content: "Explain gravity"}},
})
defer stream.Close()

for {
    chunk, err := stream.Recv()
    if err == io.EOF {
        break
    }
    for _, c := range chunk.Choices {
        if c.Delta.Content != nil {
            fmt.Print(*c.Delta.Content)
        }
    }
}
```

Custom config:
```go
cc := cchat.NewClient(&cchat.ClientConfig{
    CLIPath:       "/usr/local/bin/claude",
    Model:         "opus",
    MaxConcurrent: 8,
})
client := oai.NewClient(cc)
```

---

## Architecture

```
cc-sdk-go/
├── ccwire/       # Wire format types + NDJSON parser
├── cchat/        # Subprocess wrapper, concurrency control
├── oai/          # OpenAI types + bridge (OAI <-> CC translation)
├── server/       # HTTP server, SSE, middleware (auth, logging, panic recovery)
└── cmd/
    ├── cc-proxy/ # The proxy binary
    └── cc-repl/  # Interactive REPL for testing
```

Dependency flow is strictly left-to-right: `server -> oai -> cchat -> ccwire`. No cycles. No frameworks. Pure stdlib (one indirect dep for nanoid generation).

### Design decisions worth knowing

- **Stdin pipe for prompts** — Conversation history goes through stdin, not CLI args. No OS argument length limits.
- **Full system prompt replacement** — `--tools=""` disables CC's built-in tools, `--system-prompt` replaces the default. Deterministic prompts, better cache hits.
- **Streaming safety margin** — With tools enabled, the last 11 bytes are withheld from the stream to prevent partial `<tool_call>` XML tags from leaking as text. On completion, the full response is parsed authoritatively.
- **Channel semaphore** — `MaxConcurrent` is enforced via a buffered channel. Each request gets its own isolated process.
- **Constant-time auth** — API key comparison uses `crypto/subtle` to prevent timing attacks.

---

## Limitations

Claude Code CLI doesn't expose standard sampling parameters. These request fields are **accepted but silently ignored**:

`temperature`, `top_p`, `max_tokens`, `max_completion_tokens`, `stop`, `n`

Functional fields: `model`, `messages`, `tools`, `stream`

`effort` (low/medium/high) is supported on the `oai.Client`:
```go
client := oai.NewClientDefault()
client.Effort = oai.EffortLow
```

---

## License

MIT
