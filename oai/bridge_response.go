package oai

import (
	"fmt"
	"strings"
	"time"

	"github.com/codewandler/cc-sdk-go/ccwire"
)

// ResultToResponse converts Claude Code wire messages into an OpenAI-compatible
// [ChatCompletionResponse]. It takes the final [ccwire.ResultMessage] and the
// last [ccwire.AssistantMessage] (which may be nil if only a result was received).
//
// When hasTools is true, the response text is scanned for <tool_call> XML tags
// using [ParseToolCalls]. If tool calls are found, the response's FinishReason
// is set to "tool_calls"; otherwise it is "stop".
//
// Token usage is derived from the result's Usage field, with all input token
// categories (direct, cache-read, cache-creation) summed into PromptTokens.
func ResultToResponse(result *ccwire.ResultMessage, assistant *ccwire.AssistantMessage, hasTools bool) *ChatCompletionResponse {
	resp := &ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", result.SessionID),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelFromResult(result, assistant),
	}

	// Build message content from assistant message or result text
	var text string
	if assistant != nil {
		text = extractText(assistant)
	} else {
		text = result.Result
	}

	msg := ChatMessage{
		Role: "assistant",
	}
	finishReason := "stop"

	if hasTools {
		cleanText, toolCalls := ParseToolCalls(text)
		if len(toolCalls) > 0 {
			msg.ToolCalls = toolCalls
			finishReason = "tool_calls"
		}
		if cleanText != "" {
			msg.Content = cleanText
		}
	} else {
		msg.Content = text
	}

	resp.Choices = []Choice{
		{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		},
	}

	resp.Usage = usageFromResult(result)

	return resp
}

func extractText(assistant *ccwire.AssistantMessage) string {
	var builder strings.Builder
	for _, block := range assistant.Message.Content {
		if block.Type == "text" {
			builder.WriteString(block.Text)
		}
	}
	return builder.String()
}

func modelFromResult(result *ccwire.ResultMessage, assistant *ccwire.AssistantMessage) string {
	if assistant != nil && assistant.Message.Model != "" {
		return assistant.Message.Model
	}
	// Try to extract from modelUsage
	for model := range result.ModelUsage {
		return model
	}
	return "unknown"
}

func usageFromResult(result *ccwire.ResultMessage) *Usage {
	return &Usage{
		PromptTokens:     result.Usage.InputTokens + result.Usage.CacheReadInputTokens + result.Usage.CacheCreationInputTokens,
		CompletionTokens: result.Usage.OutputTokens,
		TotalTokens:      result.Usage.InputTokens + result.Usage.CacheReadInputTokens + result.Usage.CacheCreationInputTokens + result.Usage.OutputTokens,
	}
}
