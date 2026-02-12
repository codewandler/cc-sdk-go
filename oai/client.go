package oai

import (
	"context"
	"fmt"
	"io"

	"github.com/codewandler/cc-sdk-go/cchat"
	"github.com/codewandler/cc-sdk-go/ccwire"
)

// Effort controls the thinking effort level passed to the Claude Code CLI
// via the --effort flag. The zero value means no flag is passed, which lets
// Claude Code use its default effort level.
type Effort string

const (
	// EffortLow requests minimal thinking effort.
	EffortLow Effort = "low"
	// EffortMedium requests moderate thinking effort.
	EffortMedium Effort = "medium"
	// EffortHigh requests maximum thinking effort.
	EffortHigh Effort = "high"
)

func (e Effort) validate() error {
	switch e {
	case "", EffortLow, EffortMedium, EffortHigh:
		return nil
	default:
		return fmt.Errorf("invalid effort %q: must be low, medium, or high", e)
	}
}

// Model represents an OpenAI-compatible model descriptor, as returned by
// [Client.ListModels]. ID contains the model name (e.g. "sonnet", "opus"),
// Object is always "model", and OwnedBy is "anthropic".
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// APIError is returned by [Client] methods when a request fails. Type indicates
// the error category: "invalid_request_error" for validation failures,
// "service_unavailable" when the Claude Code CLI cannot be started,
// "internal_error" for stream read failures, and "claude_error" when the
// Claude Code process itself reports an error.
type APIError struct {
	Message string
	Type    string
	Code    string
}

// Error implements the error interface, returning the error message.
func (e *APIError) Error() string { return e.Message }

// Client provides an OpenAI-compatible programmatic interface backed by
// [cchat.Client]. It can be used directly in Go programs without starting an
// HTTP server. Each call to [Client.CreateChatCompletion] or
// [Client.CreateChatCompletionStream] spawns an isolated claude CLI process
// via the underlying cchat.Client.
//
// The Client bridges OAI concepts to Claude Code concepts:
//   - Model names (e.g. "sonnet") are passed through to the CLI's --model flag.
//   - Conversation messages are flattened into a role-prefixed prompt string.
//   - Tool definitions are injected into the system prompt as Markdown instructions.
//   - The Effort field maps to the CLI's --effort flag.
type Client struct {
	cc *cchat.Client

	// Effort sets the --effort flag for all requests.
	// Use EffortLow, EffortMedium, or EffortHigh.
	// Zero value means no flag is passed (Claude Code default).
	Effort Effort
}

// NewClient creates a [Client] that wraps the given [cchat.Client].
// Use this constructor when you need to customize the cchat.ClientConfig
// (e.g. CLI path, max concurrency, or working directory).
func NewClient(cc *cchat.Client) *Client {
	return &Client{cc: cc}
}

// NewClientDefault creates a [Client] with sensible defaults: the "claude"
// CLI from PATH and a maximum concurrency of 64. This is the simplest way to
// get started.
func NewClientDefault() *Client {
	return NewClient(cchat.NewClient(&cchat.ClientConfig{
		CLIPath:       "claude",
		MaxConcurrent: 64,
	}))
}

// ListModels returns a static list of available Claude model identifiers
// (sonnet, opus, haiku). The context parameter is accepted for API consistency
// but is not used. The returned error is always nil.
func (c *Client) ListModels(_ context.Context) ([]Model, error) {
	return []Model{
		{ID: "sonnet", Object: "model", OwnedBy: "anthropic"},
		{ID: "opus", Object: "model", OwnedBy: "anthropic"},
		{ID: "haiku", Object: "model", OwnedBy: "anthropic"},
	}, nil
}

// CreateChatCompletion sends a non-streaming chat completion request to the
// Claude Code CLI and blocks until the full response is available. The request's
// Stream field is forced to false regardless of its input value.
//
// It returns an [*APIError] on failure. Possible error types are
// "invalid_request_error" (bad Effort value), "service_unavailable" (CLI
// spawn failure), "internal_error" (stream read error or missing result),
// and "claude_error" (the CLI reported an error).
func (c *Client) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if err := c.Effort.validate(); err != nil {
		return nil, &APIError{Message: err.Error(), Type: "invalid_request_error"}
	}
	req.Stream = false
	prompt, opts := RequestToQuery(&req)
	opts.Effort = string(c.Effort)

	stream, err := c.cc.Query(ctx, prompt, opts)
	if err != nil {
		return nil, &APIError{Message: err.Error(), Type: "service_unavailable"}
	}
	defer stream.Close()

	var lastAssistant *ccwire.AssistantMessage
	var result *ccwire.ResultMessage

	for {
		msg, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, &APIError{Message: err.Error(), Type: "internal_error"}
		}
		switch m := msg.(type) {
		case *ccwire.AssistantMessage:
			lastAssistant = m
		case *ccwire.ResultMessage:
			result = m
		}
	}

	if result == nil {
		return nil, &APIError{Message: "no result received from claude", Type: "internal_error"}
	}
	if result.IsError {
		return nil, &APIError{Message: result.Result, Type: "claude_error"}
	}

	return ResultToResponse(result, lastAssistant, len(req.Tools) > 0), nil
}
