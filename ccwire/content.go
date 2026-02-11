package ccwire

import "encoding/json"

// ContentBlock represents a single content block in an assistant message.
// The Type field determines which other fields are populated.
type ContentBlock struct {
	Type string `json:"type"` // "text", "thinking", "tool_use", "tool_result"

	// Text block fields
	Text string `json:"text,omitempty"`

	// Thinking block fields
	Thinking string `json:"thinking,omitempty"`

	// Tool use block fields
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`

	// Tool result block fields
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// StreamEvent represents a parsed stream event with typed access to common event types.
type StreamEvent struct {
	Type string // "message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"
	Raw  map[string]any
}

// ParseStreamEvent extracts a typed StreamEvent from a StreamEventMessage.
func ParseStreamEvent(msg *StreamEventMessage) StreamEvent {
	ev := StreamEvent{Raw: msg.Event}
	if t, ok := msg.Event["type"].(string); ok {
		ev.Type = t
	}
	return ev
}

// DeltaText extracts the text from a content_block_delta event.
// Returns empty string if the event is not a text_delta.
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

// Index returns the content block index from the event, or -1 if not present.
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

// StopReason extracts the stop_reason from a message_delta event.
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
