package oai

// ChatCompletionResponse represents an OpenAI-compatible chat completion response.
// It is produced by [ResultToResponse] from Claude Code wire messages, or by
// [Client.CreateChatCompletion]. The ID is derived from the Claude Code session ID,
// and Model reflects the actual model used by the Claude backend.
type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // "chat.completion"
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

// Choice represents a single completion alternative in the response.
// FinishReason indicates why generation stopped: "stop" for normal completion,
// "tool_calls" when the model invoked one or more tools, or "length" if the
// output was truncated due to token limits.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"` // "stop", "tool_calls", "length"
}

// Usage contains token usage statistics for a completion request.
// PromptTokens includes all input tokens (direct, cache-read, and cache-creation).
// CompletionTokens reflects the output tokens. TotalTokens is their sum.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ErrorResponse represents an OpenAI-compatible error response body.
// It wraps an [ErrorDetail] and is intended for JSON serialization in HTTP responses.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error information within an [ErrorResponse].
// Type categorizes the error (e.g. "invalid_request_error", "internal_error").
// Code is an optional machine-readable error code.
type ErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Code    *string `json:"code,omitempty"`
}
