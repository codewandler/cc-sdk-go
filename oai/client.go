package oai

import (
	"context"
	"fmt"
	"io"

	"github.com/codewandler/cc-sdk-go/cchat"
	"github.com/codewandler/cc-sdk-go/ccwire"
)

// Effort controls the thinking effort level for Claude Code.
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

func (e Effort) validate() error {
	switch e {
	case "", EffortLow, EffortMedium, EffortHigh:
		return nil
	default:
		return fmt.Errorf("invalid effort %q: must be low, medium, or high", e)
	}
}

// Model represents an OpenAI-compatible model object.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// APIError is returned when the Claude Code process reports an error.
type APIError struct {
	Message string
	Type    string
	Code    string
}

func (e *APIError) Error() string { return e.Message }

// Client provides an embedded OpenAI-compatible interface backed by cchat.Client.
// No HTTP server required — calls bridge logic and cchat directly.
type Client struct {
	cc *cchat.Client

	// Effort sets the --effort flag for all requests.
	// Use EffortLow, EffortMedium, or EffortHigh.
	// Zero value means no flag is passed (Claude Code default).
	Effort Effort
}

// NewClient wraps an existing cchat.Client.
func NewClient(cc *cchat.Client) *Client {
	return &Client{cc: cc}
}

// NewClientDefault creates a Client with sensible defaults (CLIPath: "claude").
func NewClientDefault() *Client {
	return NewClient(cchat.NewClient(&cchat.ClientConfig{
		CLIPath:       "claude",
		MaxConcurrent: 64,
	}))
}

// ListModels returns the static list of available Claude models.
func (c *Client) ListModels(_ context.Context) ([]Model, error) {
	return []Model{
		{ID: "sonnet", Object: "model", OwnedBy: "anthropic"},
		{ID: "opus", Object: "model", OwnedBy: "anthropic"},
		{ID: "haiku", Object: "model", OwnedBy: "anthropic"},
	}, nil
}

// CreateChatCompletion sends a non-streaming chat completion request.
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
