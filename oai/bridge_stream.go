package oai

import (
	"fmt"
	"strings"
	"time"

	"github.com/codewandler/cc-sdk-go/ccwire"
)

// tagMaxPrefix is the length of "<tool_call>" — we hold back this many bytes
// from the end of the buffer to avoid streaming partial tag prefixes.
const tagMaxPrefix = len("<tool_call>")

// StreamState tracks state during streaming translation.
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
func NewStreamState(hasTools bool) *StreamState {
	return &StreamState{
		ID:       fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Created:  time.Now().Unix(),
		HasTools: hasTools,
	}
}

// InitChunk creates the initial SSE chunk with the role field.
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

// TextDeltaChunk processes a text delta event.
// Returns nil if nothing should be emitted yet.
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

// FinishChunk creates the final chunk(s) with finish_reason.
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

// HandleStreamEvent processes a CC stream event and returns OAI chunks to emit.
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
