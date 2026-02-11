// Package oai defines OpenAI-compatible chat completion request/response types.
package oai

import "encoding/json"

// ChatCompletionRequest represents an OpenAI-compatible chat completion request.
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

// ChatMessage represents a message in the conversation.
type ChatMessage struct {
	Role       string     `json:"role"` // "system", "user", "assistant", "tool"
	Content    any        `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// StringContent returns the message content as a string.
// Handles both string content and array-of-parts content.
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

// ContentPart represents a part of a multi-part message content.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Tool represents a tool definition in an OpenAI request.
type Tool struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition describes a function that the model can call.
type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolCall represents a tool call made by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the function name and arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
