# cc-sdk-go

Use Claude Code as an OpenAI-compatible inference proxy. Any framework or tool that speaks the OpenAI chat completions protocol can use Claude Code as its backend.

Your framework handles the agentic loop, tool execution, and state. Claude Code is purely the inference layer.

## 🔓 The Problem This Solves

**Anthropic has shut down API token extraction from Claude Code CLI.** Tools that previously extracted tokens for use in third-party applications (like open-code, claude-coder, and custom scripts) are now broken.

`cc-sdk-go` spawns official `claude` CLI subprocesses via the public SDK and exposes them as an **OpenAI-compatible `/v1/chat/completions` endpoint**.

```bash
# Run the proxy
cc-proxy -addr=:8080

# Any OpenAI-compatible tool now works with Claude
curl http://localhost:8080/v1/chat/completions \
  -d '{"model":"sonnet","messages":[{"role":"user","content":"Hello!"}]}'
```

## ✨ Why This Approach?

| | Token Extraction (Broken) | cc-sdk-go (This Tool) |
|---|---|---|
| **Method** | Reverse-engineering internal storage | Official CLI subprocesses |
| **ToS Status** | ⚠️ Violates Anthropic Terms | ✅ Fully compliant |
| **Setup** | Complex extraction scripts | Just `claude setup-token` |
| **API Features** | Full Anthropic API (temp, tokens, etc.) | Limited to CLI capabilities |

**Trade-off**: You lose fine-grained control over sampling parameters (temperature, max_tokens, etc.) but gain ToS-compliant access to Claude's models via the official CLI.

**Your app handles the agentic loop. Claude Code handles the inference. No token extraction required.**

## How it works

```
Your app  →  POST /v1/chat/completions  →  cc-proxy
                                              ↓
                                         translate OAI → CC format
                                              ↓
                                         claude --print --tools="" (subprocess)
                                              ↓
                                         NDJSON stdout → parse → stream back as SSE
```

Each request is stateless: spawns a `claude` process, pipes the prompt via stdin, streams back the response, process exits. CC's built-in tools are disabled (`--tools=""`), and the system prompt is fully replaced via `--system-prompt`.

## Quick start

```bash
go install github.com/codewandler/cc-sdk-go/cmd/cc-proxy@latest

cc-proxy -addr=:8080 -model=sonnet
```

Then use it like any OpenAI endpoint:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sonnet",
    "messages": [{"role": "user", "content": "What is 2+2?"}],
    "stream": true
  }'
```

## Prerequisites

- Go 1.25+
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) installed and authenticated (`claude setup-token`)

## Usage

### CLI flags

```
cc-proxy [flags]

  -addr string          Listen address (default ":8080")
  -model string         Default model (e.g. sonnet, opus, haiku)
  -api-key string       API key for Bearer auth (empty = no auth)
  -claude-path string   Path to claude binary (default "claude")
  -max-concurrent int   Max concurrent claude processes (0 = unlimited)
  -timeout duration     Per-request timeout (default 5m)
  -work-dir string      Working directory for claude processes
```

The API key can also be set via the `CC_PROXY_API_KEY` environment variable.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/chat/completions` | Chat completion (streaming and non-streaming) |
| GET | `/v1/models` | List available models |

### Tool calls

When you include `tools` in your request, cc-proxy injects tool definitions into the system prompt and instructs the model to output `<tool_call>` XML tags. These are parsed from the response and returned as structured `tool_calls` in the OpenAI format.

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

Response:

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "I'll check the weather in NYC for you.",
      "tool_calls": [{
        "id": "call_0",
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\":\"NYC\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

### Using with OpenAI client libraries

Python:

```python
from openai import OpenAI

client = OpenAI(base_url="http://localhost:8080/v1", api_key="any")
resp = client.chat.completions.create(
    model="sonnet",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(resp.choices[0].message.content)
```

Go:

```go
import "github.com/sashabaranov/go-openai"

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

## Using as a Go library

The SDK can be used directly without the HTTP server:

```go
import (
    "context"
    "fmt"

    "github.com/codewandler/cc-sdk-go/cchat"
)

client := cchat.NewClient(&cchat.ClientConfig{
    Model:         "sonnet",
    MaxConcurrent: 4,
})

stream, err := client.Query(ctx, "What is 2+2?", cchat.QueryOptions{
    SystemPrompt: "You are a helpful math tutor.",
})
if err != nil {
    // handle error
}
defer stream.Close()

result, err := stream.Result()
fmt.Println(result.Result) // "4"
```

For streaming, iterate over messages:

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
    // handle msg based on type: *ccwire.StreamEventMessage, *ccwire.AssistantMessage, etc.
}
```

### OpenAI-compatible client (`oai.Client`)

If you prefer the OpenAI request/response types without running the HTTP server, use `oai.Client` directly:

```go
import (
    "context"
    "fmt"

    "github.com/codewandler/cc-sdk-go/oai"
)

client := oai.NewClientDefault()

resp, err := client.CreateChatCompletion(ctx, oai.ChatCompletionRequest{
    Model: "sonnet",
    Messages: []oai.ChatMessage{
        {Role: "system", Content: "You are a helpful assistant."},
        {Role: "user", Content: "What is 2+2?"},
    },
})
if err != nil {
    // handle error
}
fmt.Println(resp.Choices[0].Message.StringContent())
```

Streaming:

```go
stream, err := client.CreateChatCompletionStream(ctx, oai.ChatCompletionRequest{
    Model: "sonnet",
    Messages: []oai.ChatMessage{
        {Role: "user", Content: "Explain gravity"},
    },
})
if err != nil {
    // handle error
}
defer stream.Close()

for {
    chunk, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        // handle error
    }
    for _, c := range chunk.Choices {
        if c.Delta.Content != nil {
            fmt.Print(*c.Delta.Content)
        }
    }
}
```

For custom configuration, wrap an existing `cchat.Client`:

```go
cc := cchat.NewClient(&cchat.ClientConfig{
    CLIPath:       "/usr/local/bin/claude",
    Model:         "opus",
    MaxConcurrent: 8,
})
client := oai.NewClient(cc)
```

## Package structure

```
cc-sdk-go/
├── ccwire/       # CC CLI wire format types + NDJSON parser
├── cchat/        # Core SDK: wraps CC CLI subprocess
├── oai/          # OpenAI types + bridge (OpenAI ↔ CC translation)
├── server/       # HTTP server + middleware
└── cmd/cc-proxy/ # CLI entry point
```

## Design notes

**Stdin pipe for prompts** — The conversation history is written to claude's stdin (then closed), avoiding OS argument length limits for long conversations.

**Full system prompt replacement** — Since `--tools=""` disables all CC tools, the default CC system prompt is irrelevant. We replace it entirely via `--system-prompt`, giving deterministic prompts and better cache hit rates.

**Streaming with tools** — Text is streamed normally. A safety margin holds back the last few characters to prevent partial `<tool_call>` XML tags from leaking into the text stream. When the response completes, tool calls are parsed authoritatively from the full text and emitted as a structured chunk.

**Concurrency** — `MaxConcurrent` is enforced via a buffered channel semaphore. Each request gets its own isolated claude process.

## Limitations

Claude Code CLI does not expose standard OpenAI sampling parameters. Only the following request fields are functional:

- `model` — mapped to `--model`
- `tools` — injected into the system prompt
- `messages` — the conversation history

Parameters like `temperature`, `top_p`, `max_tokens`, `max_completion_tokens`, `stop`, and `n` are accepted in the request but silently ignored.

`effort` (low/medium/high) is supported but not as a request field — set it on the `oai.Client`:

```go
client := oai.NewClientDefault()
client.Effort = oai.EffortLow
```

## License

MIT
