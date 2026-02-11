package oai

import (
	"fmt"
	"time"

	"github.com/codewandler/cc-sdk-go/ccwire"
)

// ResultToResponse converts a CC result and assistant message into an OpenAI chat completion response.
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
	for _, block := range assistant.Message.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
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
