package ccwire

import "encoding/json"

// ContentBlock represents a single content block in an assistant message.
// The Type field determines which other fields are populated:
//
//   - Type "text": Text is populated with the generated text content.
//   - Type "thinking": Thinking is populated with the model's chain-of-thought.
//   - Type "tool_use": ID, Name, and Input are populated. ID is the unique
//     tool-use block identifier, Name is the tool name, and Input contains
//     the tool invocation arguments.
//   - Type "tool_result": ToolUseID, Content, and IsError are populated.
//     ToolUseID references the corresponding tool_use block, Content holds
//     the tool output, and IsError indicates whether the tool returned an error.
//
// Fields not relevant to the block's Type are omitted (zero-valued).
type ContentBlock struct {
	// Type identifies the content block kind. One of "text", "thinking",
	// "tool_use", or "tool_result".
	Type string `json:"type"`

	// Text contains the generated text. Populated only for "text" blocks.
	Text string `json:"text,omitempty"`

	// Thinking contains the model's chain-of-thought reasoning.
	// Populated only for "thinking" blocks.
	Thinking string `json:"thinking,omitempty"`

	// ID is the unique identifier for a tool-use block.
	// Populated only for "tool_use" blocks.
	ID string `json:"id,omitempty"`

	// Name is the name of the tool being invoked.
	// Populated only for "tool_use" blocks.
	Name string `json:"name,omitempty"`

	// Input contains the tool invocation arguments as key-value pairs.
	// Populated only for "tool_use" blocks.
	Input map[string]any `json:"input,omitempty"`

	// ToolUseID references the ID of the corresponding "tool_use" block.
	// Populated only for "tool_result" blocks.
	ToolUseID string `json:"tool_use_id,omitempty"`

	// Content holds the tool's output text.
	// Populated only for "tool_result" blocks.
	Content string `json:"content,omitempty"`

	// IsError indicates whether the tool invocation returned an error.
	// Populated only for "tool_result" blocks.
	IsError bool `json:"is_error,omitempty"`
}

// StreamEvent represents a parsed streaming event with typed accessor methods
// for common event fields. It is produced by [ParseStreamEvent] from a raw
// [StreamEventMessage].
type StreamEvent struct {
	// Type is the event type string. Common values are "message_start",
	// "content_block_start", "content_block_delta", "content_block_stop",
	// "message_delta", and "message_stop".
	Type string

	// Raw contains the full event data as parsed JSON key-value pairs.
	// Numeric values may be represented as [json.Number] to preserve precision.
	Raw map[string]any
}

// ParseStreamEvent extracts a typed [StreamEvent] from a [StreamEventMessage].
// It reads the "type" field from the event's raw data. If the "type" field is
// missing or not a string, the returned StreamEvent will have an empty Type.
func ParseStreamEvent(msg *StreamEventMessage) StreamEvent {
	ev := StreamEvent{Raw: msg.Event}
	if t, ok := msg.Event["type"].(string); ok {
		ev.Type = t
	}
	return ev
}

// DeltaText extracts the incremental text from a content_block_delta event
// whose delta type is "text_delta". It returns the text string from the
// delta's "text" field.
//
// For events that are not content_block_delta, or for delta types other than
// "text_delta" (e.g., "input_json_delta"), DeltaText returns an empty string.
func (e StreamEvent) DeltaText() string {
	delta, ok := e.Raw["delta"].(map[string]any)
	if !ok {
		return ""
	}
	if dt, ok := delta["type"].(string); !ok || dt != "text_delta" {
		return ""
	}
	if text, ok := delta["text"].(string); ok {
		return text
	}
	return ""
}

// Index returns the zero-based content block index from the event. This field
// is present on content_block_start, content_block_delta, and
// content_block_stop events.
//
// Returns -1 if the event does not contain an "index" field or if the value
// cannot be converted to an integer. The index value may be stored as either
// a [json.Number] or a float64, and both representations are handled.
func (e StreamEvent) Index() int {
	if idx, ok := e.Raw["index"].(json.Number); ok {
		if n, err := idx.Int64(); err == nil {
			return int(n)
		}
	}
	if idx, ok := e.Raw["index"].(float64); ok {
		return int(idx)
	}
	return -1
}

// StopReason extracts the stop_reason from a message_delta event's "delta"
// object. Common values include "end_turn" and "max_tokens".
//
// For events that are not message_delta, or when the delta object does not
// contain a "stop_reason" field, StopReason returns an empty string.
func (e StreamEvent) StopReason() string {
	delta, ok := e.Raw["delta"].(map[string]any)
	if !ok {
		return ""
	}
	if reason, ok := delta["stop_reason"].(string); ok {
		return reason
	}
	return ""
}
