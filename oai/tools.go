package oai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ToolCallInstructions generates system prompt text that instructs the model
// how to use the provided tools via <tool_call> XML tags.
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

// ParseToolCalls extracts tool calls from response text containing <tool_call> tags.
// Returns the text with tool calls removed, and the parsed tool calls.
func ParseToolCalls(text string) (cleanText string, calls []ToolCall) {
	matches := toolCallRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	var clean strings.Builder
	lastEnd := 0

	for i, match := range matches {
		// match[0:1] = full match start/end, match[2:3] = capture group start/end
		clean.WriteString(text[lastEnd:match[0]])
		lastEnd = match[1]

		jsonStr := text[match[2]:match[3]]
		var parsed struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			continue
		}

		argsJSON, err := json.Marshal(parsed.Arguments)
		if err != nil {
			continue
		}

		calls = append(calls, ToolCall{
			ID:   fmt.Sprintf("call_%d", i),
			Type: "function",
			Function: FunctionCall{
				Name:      parsed.Name,
				Arguments: string(argsJSON),
			},
		})
	}

	clean.WriteString(text[lastEnd:])
	cleanText = strings.TrimSpace(clean.String())
	return cleanText, calls
}

// HasToolCallPrefix checks if text contains a <tool_call> opening tag (complete or partial).
func HasToolCallPrefix(text string) bool {
	return toolCallRe.MatchString(text) || strings.Contains(text, "<tool_call")
}
