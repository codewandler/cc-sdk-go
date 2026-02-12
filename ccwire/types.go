// Package ccwire defines the wire format types and a streaming NDJSON parser
// for Claude Code CLI output.
//
// When the Claude Code CLI is invoked with --output-format=stream-json, it
// writes newline-delimited JSON (NDJSON) to stdout. Each line is a JSON object
// with a "type" field that discriminates between four message kinds:
//
//   - "system" ([SystemMessage]): Session metadata, emitted first.
//   - "assistant" ([AssistantMessage]): The model's response with content blocks.
//   - "stream_event" ([StreamEventMessage]): Incremental streaming events such
//     as content_block_delta and message_delta.
//   - "result" ([ResultMessage]): Final summary with aggregated usage and cost,
//     emitted last.
//
// All message types implement the [Message] interface, which provides type
// discrimination via [Message.MsgType].
//
// Use [NewParser] to create a streaming parser that reads NDJSON lines and
// returns typed messages:
//
//	p := ccwire.NewParser(stdout)
//	for {
//	    msg, err := p.Next()
//	    if err == io.EOF {
//	        break
//	    }
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    switch m := msg.(type) {
//	    case *ccwire.SystemMessage:
//	        fmt.Println("session:", m.SessionID)
//	    case *ccwire.StreamEventMessage:
//	        ev := ccwire.ParseStreamEvent(m)
//	        fmt.Print(ev.DeltaText())
//	    case *ccwire.ResultMessage:
//	        fmt.Println("tokens:", m.Usage.InputTokens+m.Usage.OutputTokens)
//	    }
//	}
//
// This is the lowest-level package in the cc-sdk dependency chain. It has no
// dependencies outside the Go standard library.
package ccwire

// MessageType identifies the kind of NDJSON message emitted by the Claude Code
// CLI. Each line of output contains a "type" field whose value corresponds to
// one of the constants below.
type MessageType string

const (
	// TypeSystem identifies a [SystemMessage], which carries session metadata
	// and is the first message emitted on each invocation.
	TypeSystem MessageType = "system"

	// TypeAssistant identifies an [AssistantMessage], which contains the
	// model's response including content blocks and token usage.
	TypeAssistant MessageType = "assistant"

	// TypeResult identifies a [ResultMessage], which is the final message
	// emitted and contains aggregated usage, cost, and the overall result text.
	TypeResult MessageType = "result"

	// TypeStreamEvent identifies a [StreamEventMessage], which wraps an
	// incremental streaming event such as content_block_delta or message_delta.
	TypeStreamEvent MessageType = "stream_event"
)

// Message is the common interface implemented by all Claude Code wire message
// types. Use a type switch on the concrete type to access message-specific
// fields.
type Message interface {
	// MsgType returns the [MessageType] that identifies this message kind.
	MsgType() MessageType
}

// SystemMessage is the first message emitted when a Claude Code session starts.
// It contains session metadata including the session identifier, the model
// being used, the working directory, and the list of available tools.
type SystemMessage struct {
	// Subtype is the system message subtype (e.g., "init").
	Subtype string `json:"subtype"`

	// SessionID is the unique identifier for this Claude Code session.
	SessionID string `json:"session_id"`

	// Model is the name of the Claude model being used (e.g., "sonnet").
	Model string `json:"model"`

	// CWD is the working directory of the Claude Code process.
	CWD string `json:"cwd"`

	// Tools lists the names of tools available to the model in this session.
	Tools []string `json:"tools"`
}

// MsgType returns [TypeSystem].
func (m *SystemMessage) MsgType() MessageType { return TypeSystem }

// AssistantMessage contains the model's response, including content blocks,
// token usage, and optional parent tool-use context. It is emitted after the
// model generates a complete response (in non-streaming mode) or alongside
// streaming events.
type AssistantMessage struct {
	// Message holds the nested response object with content blocks and usage.
	Message AssistantInner `json:"message"`

	// SessionID is the unique identifier for this Claude Code session.
	SessionID string `json:"session_id"`

	// ParentToolUseID, when non-nil, indicates this response was generated in
	// the context of a tool-use turn and references the parent tool_use block ID.
	ParentToolUseID *string `json:"parent_tool_use_id"`
}

// MsgType returns [TypeAssistant].
func (m *AssistantMessage) MsgType() MessageType { return TypeAssistant }

// AssistantInner is the nested message object within an [AssistantMessage].
// It mirrors the Anthropic API message structure with content blocks, stop
// reason, and token usage.
type AssistantInner struct {
	// ID is the unique message identifier assigned by the API.
	ID string `json:"id"`

	// Type is the message object type (typically "message").
	Type string `json:"type"`

	// Role is the message role (typically "assistant").
	Role string `json:"role"`

	// Model is the model that generated this response.
	Model string `json:"model"`

	// Content contains the ordered list of content blocks in the response.
	// See [ContentBlock] for per-type field documentation.
	Content []ContentBlock `json:"content"`

	// StopReason indicates why the model stopped generating. Common values
	// include "end_turn" and "max_tokens". Nil if not yet stopped.
	StopReason *string `json:"stop_reason"`

	// StopSequence is the specific stop sequence that triggered the stop,
	// if any. Nil when the stop was not caused by a stop sequence.
	StopSequence *string `json:"stop_sequence"`

	// Usage contains token counts for this message.
	Usage Usage `json:"usage"`
}

// ResultMessage is the final message emitted by the Claude Code CLI. It
// provides the complete result text, aggregated token usage, cost information,
// and error status. After receiving a ResultMessage, no further messages will
// be emitted on the stream.
type ResultMessage struct {
	// Subtype is the result message subtype (e.g., "result").
	Subtype string `json:"subtype"`

	// IsError is true when the session ended with an error.
	IsError bool `json:"is_error"`

	// Result is the final plain-text result of the session. When IsError is
	// true, this contains the error message.
	Result string `json:"result"`

	// DurationMS is the total wall-clock duration of the session in milliseconds.
	DurationMS int `json:"duration_ms"`

	// SessionID is the unique identifier for this Claude Code session.
	SessionID string `json:"session_id"`

	// TotalCostUSD is the estimated total cost of the session in US dollars.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// StopReason indicates why the model stopped generating. Nil if not applicable.
	StopReason *string `json:"stop_reason"`

	// Usage contains aggregated token counts for the entire session.
	Usage ResultUsage `json:"usage"`

	// ModelUsage contains per-model usage breakdown as raw key-value pairs.
	ModelUsage map[string]any `json:"modelUsage"`
}

// MsgType returns [TypeResult].
func (m *ResultMessage) MsgType() MessageType { return TypeResult }

// StreamEventMessage wraps a single incremental streaming event from the
// Claude Code CLI. The Event map contains the raw event data with a "type"
// field indicating the event kind (e.g., "message_start",
// "content_block_delta", "content_block_stop", "message_delta",
// "message_stop").
//
// Use [ParseStreamEvent] to extract a typed [StreamEvent] for convenient
// access to common fields like delta text and stop reason.
type StreamEventMessage struct {
	// Event is the raw streaming event data, keyed by field name. The "type"
	// field identifies the event kind. Numeric values may be [json.Number]
	// to preserve precision.
	Event map[string]any `json:"event"`

	// SessionID is the unique identifier for this Claude Code session.
	SessionID string `json:"session_id"`
}

// MsgType returns [TypeStreamEvent].
func (m *StreamEventMessage) MsgType() MessageType { return TypeStreamEvent }

// Usage contains token usage information for a single model response, including
// prompt caching statistics.
type Usage struct {
	// InputTokens is the number of input tokens consumed by the request.
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the number of output tokens generated by the model.
	OutputTokens int `json:"output_tokens"`

	// CacheCreationInputTokens is the number of input tokens written to the
	// prompt cache on this request.
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`

	// CacheReadInputTokens is the number of input tokens read from the
	// prompt cache on this request.
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
}

// ResultUsage contains aggregated token usage for the entire Claude Code
// session, as reported in the [ResultMessage].
type ResultUsage struct {
	// InputTokens is the total number of input tokens consumed across all
	// turns in the session.
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the total number of output tokens generated across all
	// turns in the session.
	OutputTokens int `json:"output_tokens"`

	// CacheCreationInputTokens is the total number of input tokens written to
	// the prompt cache across all turns.
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`

	// CacheReadInputTokens is the total number of input tokens read from the
	// prompt cache across all turns.
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
}
