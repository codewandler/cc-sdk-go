// Package oai provides OpenAI-compatible types, a bridge layer that translates
// between OpenAI and Claude Code formats, and an embedded [Client] that wraps
// [github.com/codewandler/cc-sdk-go/cchat.Client] with an OpenAI-shaped API.
//
// # Types
//
// The package defines request, response, and streaming chunk types that mirror
// the OpenAI chat completion API: [ChatCompletionRequest],
// [ChatCompletionResponse], and [ChatCompletionChunk]. These types can be
// serialized to and from JSON using the standard OpenAI field names.
//
// # Bridge
//
// The bridge functions translate between OAI and Claude Code representations:
//
//   - [RequestToQuery] converts an OAI request into a prompt string and
//     [cchat.QueryOptions] for the Claude Code CLI.
//   - [ResultToResponse] converts Claude Code result messages back into an OAI
//     response.
//   - [StreamState] manages the stateful translation of streaming events from
//     Claude Code into OAI-compatible streaming chunks.
//
// # Tool Calls
//
// Tool calling is implemented via system prompt injection rather than native
// tool use. [ToolCallInstructions] converts OpenAI tool definitions into
// Markdown text instructing the model to emit <tool_call> XML tags.
// [ParseToolCalls] extracts those tags from the model's text output and
// converts them back into structured [ToolCall] objects. During streaming,
// [StreamState] holds back a safety margin of bytes to prevent partial
// <tool_call> tag prefixes from leaking to the client.
//
// # Embedded Client
//
// [Client] provides a programmatic OpenAI-compatible interface without
// requiring an HTTP server. It can be used directly in Go programs:
//
//	client := oai.NewClientDefault()
//	resp, err := client.CreateChatCompletion(ctx, oai.ChatCompletionRequest{
//	    Model: "sonnet",
//	    Messages: []oai.ChatMessage{
//	        {Role: "user", Content: "Hello!"},
//	    },
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(resp.Choices[0].Message.Content)
package oai

import "encoding/json"

// ChatCompletionRequest represents an OpenAI-compatible chat completion request.
// The Model field selects the Claude model variant (e.g. "sonnet", "opus", "haiku").
// When Tools are provided, tool call instructions are injected into the system prompt
// by the bridge layer; see [ToolCallInstructions] for details.
//
// Fields like Temperature, TopP, Stop, and N are accepted for API compatibility
// but are not forwarded to the Claude Code CLI.
type ChatCompletionRequest struct {
	Model            string        `json:"model"`
	Messages         []ChatMessage `json:"messages"`
	Stream           bool          `json:"stream,omitempty"`
	Temperature      *float64      `json:"temperature,omitempty"`
	MaxTokens        *int          `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int       `json:"max_completion_tokens,omitempty"`
	Tools            []Tool        `json:"tools,omitempty"`
	ToolChoice       any           `json:"tool_choice,omitempty"`
	Stop             any           `json:"stop,omitempty"`
	TopP             *float64      `json:"top_p,omitempty"`
	N                *int          `json:"n,omitempty"`
	User             string        `json:"user,omitempty"`
}

// ChatMessage represents a single message in the conversation history.
// Role must be one of "system", "user", "assistant", or "tool".
//
// Content may be either a plain string or an array of [ContentPart] objects
// (for multi-part messages). Use [ChatMessage.StringContent] to extract the
// text regardless of which form was provided.
//
// For assistant messages that include tool invocations, ToolCalls contains
// the structured calls. For tool-role messages returning results, ToolCallID
// identifies which call this result corresponds to.
type ChatMessage struct {
	Role       string     `json:"role"` // "system", "user", "assistant", "tool"
	Content    any        `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// StringContent extracts the textual content from the message as a plain string.
// It handles both forms of the Content field: a plain JSON string and an array
// of [ContentPart] objects (in which case all parts with Type "text" are
// concatenated). Returns the empty string if Content is nil or cannot be
// interpreted.
func (m ChatMessage) StringContent() string {
	if m.Content == nil {
		return ""
	}
	switch v := m.Content.(type) {
	case string:
		return v
	default:
		// Try to extract text from content parts array
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		var parts []ContentPart
		if err := json.Unmarshal(data, &parts); err != nil {
			// Might be a plain string in JSON
			var s string
			if err := json.Unmarshal(data, &s); err != nil {
				return ""
			}
			return s
		}
		var text string
		for _, p := range parts {
			if p.Type == "text" {
				text += p.Text
			}
		}
		return text
	}
}

// ContentPart represents one element of a multi-part message content array.
// Currently only the "text" type is supported; other types (e.g. "image_url")
// are accepted but their content is ignored by [ChatMessage.StringContent].
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Tool represents a tool definition in an OpenAI chat completion request.
// Type must be "function"; other types are silently ignored by [ToolCallInstructions].
type Tool struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition describes a callable function exposed to the model.
// Name is required. Description and Parameters are optional but improve the
// model's ability to call the function correctly. Parameters is typically a
// JSON Schema object describing the function's expected arguments.
type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolCall represents a tool invocation extracted from the model's response.
// ID is a unique identifier (prefixed with "call_") generated during parsing.
// Type is always "function". These are produced by [ParseToolCalls] from
// <tool_call> XML tags in the model output.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the function name and its arguments as a raw JSON string.
// Arguments is a JSON-encoded object (e.g. `{"param": "value"}`), matching the
// OpenAI convention of returning arguments as a string rather than a parsed object.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
