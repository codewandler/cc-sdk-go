// Package ccwire defines the wire format types for Claude Code CLI NDJSON output.
package ccwire

// MessageType identifies the kind of NDJSON message emitted by Claude Code.
type MessageType string

const (
	TypeSystem      MessageType = "system"
	TypeAssistant   MessageType = "assistant"
	TypeResult      MessageType = "result"
	TypeStreamEvent MessageType = "stream_event"
)

// Message is the common interface for all CC wire messages.
type Message interface {
	MsgType() MessageType
}

// SystemMessage is the first message emitted, containing session metadata.
type SystemMessage struct {
	Subtype   string   `json:"subtype"`
	SessionID string   `json:"session_id"`
	Model     string   `json:"model"`
	CWD       string   `json:"cwd"`
	Tools     []string `json:"tools"`
}

func (m *SystemMessage) MsgType() MessageType { return TypeSystem }

// AssistantMessage contains the model's response with content blocks.
type AssistantMessage struct {
	Message         AssistantInner `json:"message"`
	SessionID       string         `json:"session_id"`
	ParentToolUseID *string        `json:"parent_tool_use_id"`
}

func (m *AssistantMessage) MsgType() MessageType { return TypeAssistant }

// AssistantInner is the nested message object within an AssistantMessage.
type AssistantInner struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   *string        `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        Usage          `json:"usage"`
}

// ResultMessage is the final message with the complete result.
type ResultMessage struct {
	Subtype      string         `json:"subtype"`
	IsError      bool           `json:"is_error"`
	Result       string         `json:"result"`
	DurationMS   int            `json:"duration_ms"`
	SessionID    string         `json:"session_id"`
	TotalCostUSD float64        `json:"total_cost_usd"`
	StopReason   *string        `json:"stop_reason"`
	Usage        ResultUsage    `json:"usage"`
	ModelUsage   map[string]any `json:"modelUsage"`
}

func (m *ResultMessage) MsgType() MessageType { return TypeResult }

// StreamEventMessage wraps a streaming event (message_start, content_block_delta, etc.).
type StreamEventMessage struct {
	Event     map[string]any `json:"event"`
	SessionID string         `json:"session_id"`
}

func (m *StreamEventMessage) MsgType() MessageType { return TypeStreamEvent }

// Usage contains token usage information.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ResultUsage extends Usage with additional fields from the result message.
type ResultUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}
