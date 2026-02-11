package oai

// ChatCompletionChunk represents a single chunk in a streaming response.
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
type ChunkChoice struct {
	Index        int        `json:"index"`
	Delta        ChunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"` // nil until final chunk
}

// ChunkDelta represents the incremental content in a streaming chunk.
type ChunkDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   *string    `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
