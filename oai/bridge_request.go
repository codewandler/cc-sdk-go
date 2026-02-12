package oai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codewandler/cc-sdk-go/cchat"
)

// RequestToQuery converts an OpenAI [ChatCompletionRequest] into a prompt string
// and [cchat.QueryOptions] suitable for [cchat.Client.Query].
//
// Messages are translated according to their role:
//   - "system" messages are concatenated into the system prompt.
//   - "user" messages are prefixed with "[user]: ".
//   - "assistant" messages are prefixed with "[assistant]: ". If the message
//     includes ToolCalls, they are re-encoded as <tool_call> XML tags.
//   - "tool" messages become "[tool_result for <call_id>]: <content>".
//
// When the request includes Tools, [ToolCallInstructions] is appended to the
// system prompt to enable prompt-engineered tool calling.
func RequestToQuery(req *ChatCompletionRequest) (prompt string, opts cchat.QueryOptions) {
	var systemParts []string
	var convParts []string

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			systemParts = append(systemParts, msg.StringContent())

		case "user":
			convParts = append(convParts, fmt.Sprintf("[user]: %s", msg.StringContent()))

		case "assistant":
			text := msg.StringContent()
			if len(msg.ToolCalls) > 0 {
				// Encode tool calls as <tool_call> tags
				var parts []string
				if text != "" {
					parts = append(parts, text)
				}
				for _, tc := range msg.ToolCalls {
					callJSON, _ := json.Marshal(map[string]any{
						"name":      tc.Function.Name,
						"arguments": json.RawMessage(tc.Function.Arguments),
					})
					parts = append(parts, fmt.Sprintf("<tool_call>%s</tool_call>", callJSON))
				}
				text = strings.Join(parts, "\n\n")
			}
			convParts = append(convParts, fmt.Sprintf("[assistant]: %s", text))

		case "tool":
			convParts = append(convParts, fmt.Sprintf("[tool_result for %s]: %s", msg.ToolCallID, msg.StringContent()))
		}
	}

	// Build system prompt
	systemPrompt := strings.Join(systemParts, "\n\n")
	if len(req.Tools) > 0 {
		systemPrompt += ToolCallInstructions(req.Tools)
	}

	opts = cchat.QueryOptions{
		SystemPrompt: systemPrompt,
		Streaming:    req.Stream,
		Model:        req.Model,
	}

	prompt = strings.Join(convParts, "\n\n")
	return prompt, opts
}
