package bridge

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codewandler/cc-sdk-go/cchat"
	"github.com/codewandler/cc-sdk-go/oai"
)

// RequestToQuery converts an OpenAI chat completion request into a CC prompt string
// and QueryOptions suitable for cchat.Client.Query().
func RequestToQuery(req *oai.ChatCompletionRequest) (prompt string, opts cchat.QueryOptions) {
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
