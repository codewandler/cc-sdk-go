package oai

// ChatCompletionChunk represents a single server-sent event in a streaming
// chat completion response. Each chunk carries incremental content in its
// Choices via [ChunkDelta]. The final chunk in a stream has a non-nil
// FinishReason in its choice. Object is always "chat.completion.chunk".
type ChatCompletionChunk struct {
	ID                string        `json:"id"`
	Object            string        `json:"object"` // "chat.completion.chunk"
	Created           int64         `json:"created"`
	Model             string        `json:"model"`
	Choices           []ChunkChoice `json:"choices"`
	Usage             *Usage        `json:"usage,omitempty"`
	SystemFingerprint string        `json:"system_fingerprint,omitempty"`
}

// ChunkChoice represents a single choice in a streaming chunk.
// FinishReason is nil for intermediate chunks and non-nil for the final chunk
// ("stop" for normal completion, "tool_calls" when tools were invoked).
type ChunkChoice struct {
	Index        int        `json:"index"`
	Delta        ChunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"` // nil until final chunk
}

// ChunkDelta represents the incremental content in a streaming chunk.
// The first delta in a stream carries the Role ("assistant"). Subsequent
// deltas carry either Content (text fragments) or ToolCalls. Content is a
// pointer so that an empty string can be distinguished from an absent field.
type ChunkDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   *string    `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
