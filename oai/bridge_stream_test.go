package oai

import (
	"testing"

	"github.com/codewandler/cc-sdk-go/ccwire"
)

func TestNewStreamState(t *testing.T) {
	tests := []struct {
		name     string
		hasTools bool
	}{
		{"with_tools", true},
		{"without_tools", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := NewStreamState(tt.hasTools)
			if ss.HasTools != tt.hasTools {
				t.Errorf("HasTools = %v, want %v", ss.HasTools, tt.hasTools)
			}
			if ss.ID == "" {
				t.Error("ID should not be empty")
			}
			if ss.Created == 0 {
				t.Error("Created timestamp should not be zero")
			}
			if ss.Buffering {
				t.Error("Buffering should start as false")
			}
			if ss.buffer.Len() != 0 {
				t.Error("buffer should start empty")
			}
			if ss.Emitted != 0 {
				t.Error("Emitted should start at 0")
			}
		})
	}
}

func TestStreamState_InitChunk(t *testing.T) {
	ss := NewStreamState(false)
	ss.Model = "test-model"

	chunk := ss.InitChunk()

	if chunk.ID != ss.ID {
		t.Errorf("chunk.ID = %q, want %q", chunk.ID, ss.ID)
	}
	if chunk.Object != "chat.completion.chunk" {
		t.Errorf("chunk.Object = %q, want %q", chunk.Object, "chat.completion.chunk")
	}
	if chunk.Model != "test-model" {
		t.Errorf("chunk.Model = %q, want %q", chunk.Model, "test-model")
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("len(chunk.Choices) = %d, want 1", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Role != "assistant" {
		t.Errorf("chunk.Choices[0].Delta.Role = %q, want %q", chunk.Choices[0].Delta.Role, "assistant")
	}
	if chunk.Choices[0].Delta.Content != nil {
		t.Error("chunk.Choices[0].Delta.Content should be nil in init chunk")
	}
}

func TestStreamState_TextDeltaChunk_NoTools(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		wantContent string
	}{
		{"simple_text", "Hello", "Hello"},
		{"empty_text", "", ""},
		{"multiline_text", "Hello\nWorld", "Hello\nWorld"},
		{"special_chars", "<>&\"'", "<>&\"'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := NewStreamState(false)
			chunk := ss.TextDeltaChunk(tt.text)

			if chunk == nil {
				t.Fatal("chunk should not be nil in no-tools mode")
			}
			if chunk.Choices[0].Delta.Content == nil {
				t.Fatal("chunk.Choices[0].Delta.Content should not be nil")
			}
			if *chunk.Choices[0].Delta.Content != tt.wantContent {
				t.Errorf("content = %q, want %q", *chunk.Choices[0].Delta.Content, tt.wantContent)
			}
		})
	}
}

func TestStreamState_TextDeltaChunk_WithTools_SafetyMargin(t *testing.T) {
	ss := NewStreamState(true)

	chunk1 := ss.TextDeltaChunk("Hello world")
	if chunk1 != nil {
		t.Fatal("first chunk should be nil (not enough text past safety margin)")
	}
	if ss.buffer.String() != "Hello world" {
		t.Errorf("buffer = %q, want %q", ss.buffer.String(), "Hello world")
	}
	if ss.Emitted != 0 {
		t.Errorf("Emitted = %d, want 0", ss.Emitted)
	}

	chunk2 := ss.TextDeltaChunk(" and more text here")
	if chunk2 == nil {
		t.Fatal("second chunk should not be nil (enough safe text now)")
	}

	expectedBuffer := "Hello world and more text here"
	if ss.buffer.String() != expectedBuffer {
		t.Errorf("buffer = %q, want %q", ss.buffer.String(), expectedBuffer)
	}

	safeEnd := len(expectedBuffer) - tagMaxPrefix
	if ss.Emitted != safeEnd {
		t.Errorf("Emitted = %d, want %d", ss.Emitted, safeEnd)
	}

	expectedContent := expectedBuffer[:safeEnd]
	if *chunk2.Choices[0].Delta.Content != expectedContent {
		t.Errorf("content = %q, want %q", *chunk2.Choices[0].Delta.Content, expectedContent)
	}
}

func TestStreamState_TextDeltaChunk_WithTools_PartialTag(t *testing.T) {
	ss := NewStreamState(true)

	ss.TextDeltaChunk("Here is the answer: <tool_ca")

	if ss.Buffering {
		t.Error("Buffering should not trigger on partial tag without full '<tool_call'")
	}

	safeText := "Here is the answer: <tool_ca"[:len("Here is the answer: <tool_ca")-tagMaxPrefix]
	if ss.Emitted != len(safeText) {
		t.Errorf("Emitted = %d, want %d (should emit up to safety margin)", ss.Emitted, len(safeText))
	}
}

func TestStreamState_TextDeltaChunk_WithTools_FullTagDetection(t *testing.T) {
	ss := NewStreamState(true)

	chunk1 := ss.TextDeltaChunk("Short ")
	if chunk1 != nil {
		t.Fatal("chunk1 should be nil (not enough text past safety margin)")
	}

	chunk2 := ss.TextDeltaChunk("<tool_call")
	if chunk2 != nil {
		t.Error("chunk2 should be nil (buffering triggered)")
	}

	if !ss.Buffering {
		t.Error("Buffering should be true after detecting '<tool_call'")
	}

	chunk3 := ss.TextDeltaChunk(">more content</tool_call>")
	if chunk3 != nil {
		t.Error("chunk3 should be nil (still buffering)")
	}
}

func TestStreamState_TextDeltaChunk_WithTools_BufferingActive(t *testing.T) {
	ss := NewStreamState(true)
	ss.Buffering = true

	chunk := ss.TextDeltaChunk("Any text at all")
	if chunk != nil {
		t.Error("chunk should be nil when Buffering is active")
	}
	if ss.buffer.String() != "Any text at all" {
		t.Errorf("buffer = %q, want %q", ss.buffer.String(), "Any text at all")
	}
}

func TestStreamState_FinishChunk_NoTools(t *testing.T) {
	ss := NewStreamState(false)

	chunks := ss.FinishChunk(nil)

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Choices[0].FinishReason == nil {
		t.Fatal("FinishReason should not be nil")
	}
	if *chunk.Choices[0].FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", *chunk.Choices[0].FinishReason, "stop")
	}
}

func TestStreamState_FinishChunk_WithTools_NoToolCalls(t *testing.T) {
	ss := NewStreamState(true)
	ss.buffer.WriteString("Just plain text response")
	ss.Emitted = 0

	chunks := ss.FinishChunk(nil)

	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2 (content + stop)", len(chunks))
	}

	contentChunk := chunks[0]
	if contentChunk.Choices[0].Delta.Content == nil {
		t.Fatal("first chunk should have content")
	}
	if *contentChunk.Choices[0].Delta.Content != "Just plain text response" {
		t.Errorf("content = %q, want %q", *contentChunk.Choices[0].Delta.Content, "Just plain text response")
	}

	finishChunk := chunks[1]
	if finishChunk.Choices[0].FinishReason == nil {
		t.Fatal("FinishReason should not be nil")
	}
	if *finishChunk.Choices[0].FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", *finishChunk.Choices[0].FinishReason, "stop")
	}
}

func TestStreamState_FinishChunk_WithTools_HasToolCalls(t *testing.T) {
	ss := NewStreamState(true)
	ss.buffer.WriteString(`Let me check that. <tool_call>{"name": "get_weather", "arguments": {"city": "Paris"}}</tool_call>`)
	ss.Emitted = 0

	chunks := ss.FinishChunk(nil)

	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2 (content + tool_calls)", len(chunks))
	}

	contentChunk := chunks[0]
	if contentChunk.Choices[0].Delta.Content == nil {
		t.Fatal("first chunk should have content")
	}
	if *contentChunk.Choices[0].Delta.Content != "Let me check that." {
		t.Errorf("content = %q, want %q", *contentChunk.Choices[0].Delta.Content, "Let me check that.")
	}

	toolCallChunk := chunks[1]
	if toolCallChunk.Choices[0].FinishReason == nil {
		t.Fatal("FinishReason should not be nil")
	}
	if *toolCallChunk.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", *toolCallChunk.Choices[0].FinishReason, "tool_calls")
	}

	toolCalls := toolCallChunk.Choices[0].Delta.ToolCalls
	if len(toolCalls) != 1 {
		t.Fatalf("len(toolCalls) = %d, want 1", len(toolCalls))
	}

	tc := toolCalls[0]
	if tc.Function.Name != "get_weather" {
		t.Errorf("tool name = %q, want %q", tc.Function.Name, "get_weather")
	}
	if tc.ID != "call_0" {
		t.Errorf("tool ID = %q, want %q", tc.ID, "call_0")
	}
}

func TestStreamState_FinishChunk_WithTools_OnlyToolCalls(t *testing.T) {
	ss := NewStreamState(true)
	ss.buffer.WriteString(`<tool_call>{"name": "test", "arguments": {}}</tool_call>`)
	ss.Emitted = 0

	chunks := ss.FinishChunk(nil)

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1 (only tool_calls, no content)", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Choices[0].FinishReason == nil {
		t.Fatal("FinishReason should not be nil")
	}
	if *chunk.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", *chunk.Choices[0].FinishReason, "tool_calls")
	}

	if len(chunk.Choices[0].Delta.ToolCalls) != 1 {
		t.Fatalf("len(toolCalls) = %d, want 1", len(chunk.Choices[0].Delta.ToolCalls))
	}
}

func TestStreamState_FinishChunk_WithTools_PartiallyEmitted(t *testing.T) {
	ss := NewStreamState(true)
	ss.buffer.WriteString("Hello world <tool_call>{\"name\": \"test\", \"arguments\": {}}</tool_call>")
	ss.Emitted = 6

	chunks := ss.FinishChunk(nil)

	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}

	contentChunk := chunks[0]
	if contentChunk.Choices[0].Delta.Content == nil {
		t.Fatal("first chunk should have remaining content")
	}

	expectedRemaining := "world"
	if *contentChunk.Choices[0].Delta.Content != expectedRemaining {
		t.Errorf("remaining content = %q, want %q", *contentChunk.Choices[0].Delta.Content, expectedRemaining)
	}
}

func TestStreamState_FinishChunk_WithTools_AlreadyFullyEmitted(t *testing.T) {
	ss := NewStreamState(true)
	ss.buffer.WriteString("<tool_call>{\"name\": \"test\", \"arguments\": {}}</tool_call>")
	ss.Emitted = 0

	chunks := ss.FinishChunk(nil)

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1 (no content to emit, just tool call)", len(chunks))
	}

	chunk := chunks[0]
	if *chunk.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", *chunk.Choices[0].FinishReason, "tool_calls")
	}
}

func TestStreamState_FinishChunk_WithTools_MultipleToolCalls(t *testing.T) {
	ss := NewStreamState(true)
	ss.buffer.WriteString(`<tool_call>{"name": "tool_a", "arguments": {}}</tool_call><tool_call>{"name": "tool_b", "arguments": {}}</tool_call>`)
	ss.Emitted = 0

	chunks := ss.FinishChunk(nil)

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}

	toolCalls := chunks[0].Choices[0].Delta.ToolCalls
	if len(toolCalls) != 2 {
		t.Fatalf("len(toolCalls) = %d, want 2", len(toolCalls))
	}

	if toolCalls[0].Function.Name != "tool_a" {
		t.Errorf("toolCalls[0].Function.Name = %q, want %q", toolCalls[0].Function.Name, "tool_a")
	}
	if toolCalls[1].Function.Name != "tool_b" {
		t.Errorf("toolCalls[1].Function.Name = %q, want %q", toolCalls[1].Function.Name, "tool_b")
	}
}

func TestStreamState_HandleStreamEvent_MessageStart(t *testing.T) {
	ss := NewStreamState(false)

	msg := &ccwire.StreamEventMessage{
		Event: map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"role":  "assistant",
				"model": "test-model-v1",
			},
		},
	}

	chunks := ss.HandleStreamEvent(msg)

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}

	if ss.Model != "test-model-v1" {
		t.Errorf("Model = %q, want %q", ss.Model, "test-model-v1")
	}

	chunk := chunks[0]
	if chunk.Choices[0].Delta.Role != "assistant" {
		t.Errorf("Delta.Role = %q, want %q", chunk.Choices[0].Delta.Role, "assistant")
	}
}

func TestStreamState_HandleStreamEvent_ContentBlockDelta(t *testing.T) {
	ss := NewStreamState(false)

	msg := &ccwire.StreamEventMessage{
		Event: map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": "Hello, world!",
			},
		},
	}

	chunks := ss.HandleStreamEvent(msg)

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Choices[0].Delta.Content == nil {
		t.Fatal("Delta.Content should not be nil")
	}
	if *chunk.Choices[0].Delta.Content != "Hello, world!" {
		t.Errorf("Delta.Content = %q, want %q", *chunk.Choices[0].Delta.Content, "Hello, world!")
	}
}

func TestStreamState_HandleStreamEvent_ContentBlockDelta_EmptyText(t *testing.T) {
	ss := NewStreamState(false)

	msg := &ccwire.StreamEventMessage{
		Event: map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": "",
			},
		},
	}

	chunks := ss.HandleStreamEvent(msg)

	if chunks != nil {
		t.Error("chunks should be nil for empty text delta")
	}
}

func TestStreamState_HandleStreamEvent_UnknownType(t *testing.T) {
	ss := NewStreamState(false)

	msg := &ccwire.StreamEventMessage{
		Event: map[string]any{
			"type": "unknown_event_type",
		},
	}

	chunks := ss.HandleStreamEvent(msg)

	if chunks != nil {
		t.Error("chunks should be nil for unknown event types")
	}
}

func TestStreamState_MultipleChunks_Accumulation(t *testing.T) {
	ss := NewStreamState(true)

	chunk1 := ss.TextDeltaChunk("A ")
	if chunk1 != nil {
		t.Error("chunk1 should be nil (not enough safe text)")
	}

	chunk2 := ss.TextDeltaChunk("B ")
	if chunk2 != nil {
		t.Error("chunk2 should be nil (still not enough)")
	}

	chunk3 := ss.TextDeltaChunk("C D E F G H")
	if chunk3 == nil {
		t.Fatal("chunk3 should not be nil (enough safe text now)")
	}

	expectedBuffer := "A B C D E F G H"
	if ss.buffer.String() != expectedBuffer {
		t.Errorf("buffer = %q, want %q", ss.buffer.String(), expectedBuffer)
	}

	safeEnd := len(expectedBuffer) - tagMaxPrefix
	if ss.Emitted != safeEnd {
		t.Errorf("Emitted = %d, want %d", ss.Emitted, safeEnd)
	}
}

func TestStreamState_SafetyMarginBoundary(t *testing.T) {
	ss := NewStreamState(true)

	textLessThanMargin := "short"
	chunk1 := ss.TextDeltaChunk(textLessThanMargin)
	if chunk1 != nil {
		t.Error("chunk1 should be nil (text shorter than safety margin)")
	}
	if ss.Emitted != 0 {
		t.Error("Emitted should still be 0")
	}

	textExactlyMargin := "<tool_call>"
	ss = NewStreamState(true)
	chunk2 := ss.TextDeltaChunk(textExactlyMargin)
	if chunk2 != nil {
		t.Error("chunk2 should be nil (text exactly safety margin length)")
	}

	textOneMoreThanMargin := "Hello world!"
	ss = NewStreamState(true)
	chunk3 := ss.TextDeltaChunk(textOneMoreThanMargin)
	if chunk3 == nil {
		t.Fatal("chunk3 should not be nil (text is margin + 1)")
	}
	expectedEmitted := len("Hello world!") - tagMaxPrefix
	if *chunk3.Choices[0].Delta.Content != "H" {
		t.Errorf("content = %q, want %q (first char only due to safety margin)", *chunk3.Choices[0].Delta.Content, "H")
	}
	if ss.Emitted != expectedEmitted {
		t.Errorf("Emitted = %d, want %d", ss.Emitted, expectedEmitted)
	}
}
