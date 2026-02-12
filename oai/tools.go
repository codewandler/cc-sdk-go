package oai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ToolCallInstructions generates Markdown-formatted system prompt text that
// instructs the model how to invoke the provided tools by outputting <tool_call>
// XML tags. Each tool with Type "function" is listed with its name, description,
// and JSON-serialized parameters schema. Returns the empty string if tools is
// empty.
//
// The generated instructions tell the model to emit tool invocations in the form:
//
//	<tool_call>{"name": "tool_name", "arguments": {"param": "value"}}</tool_call>
//
// These tags are later extracted by [ParseToolCalls].
func ToolCallInstructions(tools []Tool) string {
	if len(tools) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Available Tools\n\n")
	b.WriteString("You have access to the following tools. To call a tool, output a <tool_call> tag:\n\n")
	b.WriteString("<tool_call>{\"name\": \"tool_name\", \"arguments\": {\"param\": \"value\"}}</tool_call>\n\n")

	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		b.WriteString("### ")
		b.WriteString(tool.Function.Name)
		b.WriteString("\n")
		if tool.Function.Description != "" {
			b.WriteString(tool.Function.Description)
			b.WriteString("\n")
		}
		if tool.Function.Parameters != nil {
			params, err := json.Marshal(tool.Function.Parameters)
			if err == nil {
				b.WriteString("Parameters: ")
				b.Write(params)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("When calling tools, output only <tool_call> tags with no additional text after them.\n")
	b.WriteString("You may output text before tool calls, and you may call multiple tools.\n")

	return b.String()
}

var toolCallRe = regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)

// ParseToolCalls extracts <tool_call> XML tags from the model's response text
// using a regex and parses the JSON payload within each tag. It returns the
// cleaned text (with successfully parsed tags removed and surrounding whitespace
// trimmed) and a slice of structured [ToolCall] values.
//
// Each parsed tool call is assigned a unique ID with the prefix "call_" followed
// by a nanoid. If nanoid generation fails, a counter-based fallback is used.
//
// Malformed tags -- those whose content is not valid JSON or whose JSON does not
// match the expected {"name": ..., "arguments": ...} schema -- are silently
// preserved in the returned text, allowing the caller to see the raw output.
func ParseToolCalls(text string) (cleanText string, calls []ToolCall) {
	matches := toolCallRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	var clean strings.Builder
	lastEnd := 0

	callIndex := 0
	for _, match := range matches {
		// match[0:1] = full match start/end, match[2:3] = capture group start/end
		jsonStr := text[match[2]:match[3]]
		var parsed struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			// JSON parse failed - preserve the entire <tool_call> tag in output
			continue
		}

		argsJSON, err := json.Marshal(parsed.Arguments)
		if err != nil {
			// Arguments marshaling failed - preserve the entire <tool_call> tag in output
			continue
		}

		// Valid tool call - remove it from output text
		clean.WriteString(text[lastEnd:match[0]])
		lastEnd = match[1]

		// Generate unique ID using gonanoid
		nanoID, err := gonanoid.New()
		if err != nil {
			// Fallback to counter-based ID if nanoid generation fails
			nanoID = fmt.Sprintf("%d", callIndex)
		}

		calls = append(calls, ToolCall{
			ID:   fmt.Sprintf("call_%s", nanoID),
			Type: "function",
			Function: FunctionCall{
				Name:      parsed.Name,
				Arguments: string(argsJSON),
			},
		})
		callIndex++
	}

	clean.WriteString(text[lastEnd:])
	cleanText = strings.TrimSpace(clean.String())
	return cleanText, calls
}

// HasToolCallPrefix reports whether text contains either a complete <tool_call>...</tool_call>
// tag or a partial opening tag prefix ("<tool_call"). This is used to detect
// whether the model has begun emitting tool call output, even before the closing
// tag has arrived.
func HasToolCallPrefix(text string) bool {
	return toolCallRe.MatchString(text) || strings.Contains(text, "<tool_call")
}
