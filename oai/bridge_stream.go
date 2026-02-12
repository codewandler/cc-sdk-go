package oai

import (
	"fmt"
	"strings"
	"time"

	"github.com/codewandler/cc-sdk-go/ccwire"
)

// tagMaxPrefix is the safety margin in bytes, equal to len("<tool_call>").
// When tools are enabled, the stream buffer withholds this many bytes from
// the end to ensure that a partial "<tool_call>" prefix is never emitted to
// the client as regular text content.
const tagMaxPrefix = len("<tool_call>")

// StreamState tracks the translation state during a streaming response,
// converting Claude Code stream events into OpenAI-compatible [ChatCompletionChunk]
// values.
//
// When HasTools is false, text deltas are forwarded immediately as content chunks.
//
// When HasTools is true, all text is accumulated in an internal buffer. On each
// delta, only the "safe" portion of the buffer is emitted -- everything except
// the last [tagMaxPrefix] bytes, which might be the start of a "<tool_call>" tag.
// Once a "<tool_call" substring is detected in the buffer, Buffering is set to
// true and no further text is emitted until the stream finishes. At finish time,
// [FinishChunk] parses the complete buffer with [ParseToolCalls] to produce
// authoritative tool call chunks or flush any remaining plain text.
type StreamState struct {
	ID        string
	Model     string
	Created   int64
	HasTools  bool
	Buffering bool           // true when we've detected <tool_call in the buffer
	buffer    strings.Builder // accumulated text (always appended when HasTools)
	Emitted   int            // number of bytes of buffer already streamed to client
}

// NewStreamState creates a new StreamState for a streaming response.
// Set hasTools to true when the request includes tool definitions, which
// enables the safety-margin buffering strategy to prevent partial XML tag leaks.
func NewStreamState(hasTools bool) *StreamState {
	return &StreamState{
		ID:       fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Created:  time.Now().Unix(),
		HasTools: hasTools,
	}
}

// InitChunk creates the initial streaming chunk that carries the assistant role.
// This should be the first chunk sent to the client in a streaming response.
func (ss *StreamState) InitChunk() *ChatCompletionChunk {
	return &ChatCompletionChunk{
		ID:      ss.ID,
		Object:  "chat.completion.chunk",
		Created: ss.Created,
		Model:   ss.Model,
		Choices: []ChunkChoice{
			{
				Index: 0,
				Delta: ChunkDelta{Role: "assistant"},
			},
		},
	}
}

// TextDeltaChunk processes an incremental text delta from the Claude Code stream.
// Without tools, the text is forwarded immediately as a content chunk. With tools
// enabled, the text is appended to the internal buffer and only the safe prefix
// (excluding the last [tagMaxPrefix] bytes) is emitted. Returns nil if there is
// nothing to emit yet -- either because the safety margin has not been exceeded
// or because buffering has been activated after detecting a tool call tag prefix.
func (ss *StreamState) TextDeltaChunk(text string) *ChatCompletionChunk {
	if !ss.HasTools {
		content := text
		return ss.makeContentChunk(&content)
	}

	// Tools mode: accumulate into buffer
	ss.buffer.WriteString(text)

	if ss.Buffering {
		return nil
	}

	// Check if we've hit a tool call tag
	if strings.Contains(ss.buffer.String(), "<tool_call") {
		ss.Buffering = true
		return nil
	}

	// Emit text up to a safety margin from the end of the buffer,
	// so partial "<tool_call>" prefixes are never streamed.
	safeEnd := ss.buffer.Len() - tagMaxPrefix
	if safeEnd <= ss.Emitted {
		return nil // not enough new safe text to emit
	}

	content := ss.buffer.String()[ss.Emitted:safeEnd]
	ss.Emitted = safeEnd
	return ss.makeContentChunk(&content)
}

// FinishChunk produces the final chunk(s) that close the streaming response.
// When tools are enabled and the buffer contains text, it is parsed with
// [ParseToolCalls]. If tool calls are found, any un-emitted clean text is
// flushed first, followed by a chunk carrying the parsed [ToolCall] values
// with FinishReason "tool_calls". If no tool calls are found, any remaining
// buffered text is flushed and a "stop" finish chunk is appended.
//
// The returned slice always ends with a chunk whose FinishReason is non-nil.
func (ss *StreamState) FinishChunk(assistant *ccwire.AssistantMessage) []*ChatCompletionChunk {
	var chunks []*ChatCompletionChunk

	if ss.HasTools && ss.buffer.Len() > 0 {
		cleanText, toolCalls := ParseToolCalls(ss.buffer.String())

		if len(toolCalls) > 0 {
			// Emit any un-streamed clean text before the tool calls
			if len(cleanText) > ss.Emitted {
				remainder := cleanText[ss.Emitted:]
				chunks = append(chunks, ss.makeContentChunk(&remainder))
			}

			// Emit tool calls
			reason := "tool_calls"
			chunks = append(chunks, &ChatCompletionChunk{
				ID:      ss.ID,
				Object:  "chat.completion.chunk",
				Created: ss.Created,
				Model:   ss.Model,
				Choices: []ChunkChoice{
					{
						Index:        0,
						Delta:        ChunkDelta{ToolCalls: toolCalls},
						FinishReason: &reason,
					},
				},
			})
			return chunks
		}

		// No tool calls found — emit any remaining buffered text
		if ss.buffer.Len() > ss.Emitted {
			remainder := ss.buffer.String()[ss.Emitted:]
			chunks = append(chunks, ss.makeContentChunk(&remainder))
		}
	}

	// Normal stop
	reason := "stop"
	chunks = append(chunks, &ChatCompletionChunk{
		ID:      ss.ID,
		Object:  "chat.completion.chunk",
		Created: ss.Created,
		Model:   ss.Model,
		Choices: []ChunkChoice{
			{
				Index:        0,
				Delta:        ChunkDelta{},
				FinishReason: &reason,
			},
		},
	})
	return chunks
}

func (ss *StreamState) makeContentChunk(content *string) *ChatCompletionChunk {
	return &ChatCompletionChunk{
		ID:      ss.ID,
		Object:  "chat.completion.chunk",
		Created: ss.Created,
		Model:   ss.Model,
		Choices: []ChunkChoice{
			{
				Index: 0,
				Delta: ChunkDelta{Content: content},
			},
		},
	}
}

// setBufferForTest sets the buffer content (for testing only).
func (ss *StreamState) setBufferForTest(content string) {
	ss.buffer.Reset()
	ss.buffer.WriteString(content)
}

// HandleStreamEvent processes a single Claude Code [ccwire.StreamEventMessage]
// and returns zero or more OAI chunks to emit. It handles "message_start" events
// (extracting the model name and returning the initial role chunk) and
// "content_block_delta" events (delegating to [StreamState.TextDeltaChunk]).
// Unrecognized event types are silently ignored.
func (ss *StreamState) HandleStreamEvent(msg *ccwire.StreamEventMessage) []*ChatCompletionChunk {
	ev := ccwire.ParseStreamEvent(msg)

	switch ev.Type {
	case "message_start":
		if message, ok := ev.Raw["message"].(map[string]any); ok {
			if model, ok := message["model"].(string); ok {
				ss.Model = model
			}
		}
		return []*ChatCompletionChunk{ss.InitChunk()}

	case "content_block_delta":
		text := ev.DeltaText()
		if text == "" {
			return nil
		}
		chunk := ss.TextDeltaChunk(text)
		if chunk == nil {
			return nil
		}
		return []*ChatCompletionChunk{chunk}

	default:
		return nil
	}
}
