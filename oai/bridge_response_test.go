package oai

import (
	"testing"

	"github.com/codewandler/cc-sdk-go/ccwire"
)

func TestExtractText(t *testing.T) {
	tests := []struct {
		name     string
		assistant *ccwire.AssistantMessage
		want     string
	}{
		{
			name: "single_text_block",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "text", Text: "Hello, world!"},
					},
				},
			},
			want: "Hello, world!",
		},
		{
			name: "multiple_text_blocks",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "text", Text: "First block. "},
						{Type: "text", Text: "Second block. "},
						{Type: "text", Text: "Third block."},
					},
				},
			},
			want: "First block. Second block. Third block.",
		},
		{
			name: "text_interleaved_with_thinking",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "text", Text: "Let me think about this."},
						{Type: "thinking", Thinking: "Internal reasoning..."},
						{Type: "text", Text: " Here's my answer."},
					},
				},
			},
			want: "Let me think about this. Here's my answer.",
		},
		{
			name: "text_interleaved_with_tool_use",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "text", Text: "I'll check the weather."},
						{Type: "tool_use", ID: "call_1", Name: "get_weather"},
						{Type: "text", Text: " Done!"},
					},
				},
			},
			want: "I'll check the weather. Done!",
		},
		{
			name: "no_text_blocks",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "thinking", Thinking: "Just thinking..."},
						{Type: "tool_use", ID: "call_1", Name: "some_tool"},
					},
				},
			},
			want: "",
		},
		{
			name: "empty_content",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{},
				},
			},
			want: "",
		},
		{
			name: "text_block_with_empty_text",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "text", Text: ""},
					},
				},
			},
			want: "",
		},
		{
			name: "multiple_blocks_some_empty",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "text", Text: "Start"},
						{Type: "text", Text: ""},
						{Type: "text", Text: "End"},
					},
				},
			},
			want: "StartEnd",
		},
		{
			name: "complex_mixed_content",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "text", Text: "First, "},
						{Type: "thinking", Thinking: "pondering..."},
						{Type: "text", Text: "I'll use a tool. "},
						{Type: "tool_use", ID: "call_1", Name: "tool_a"},
						{Type: "text", Text: "Then another. "},
						{Type: "tool_use", ID: "call_2", Name: "tool_b"},
						{Type: "text", Text: "Finally done."},
					},
				},
			},
			want: "First, I'll use a tool. Then another. Finally done.",
		},
		{
			name: "text_with_newlines_and_special_chars",
			assistant: &ccwire.AssistantMessage{
				Message: ccwire.AssistantInner{
					Content: []ccwire.ContentBlock{
						{Type: "text", Text: "Line 1\n"},
						{Type: "text", Text: "Line 2\nLine 3"},
					},
				},
			},
			want: "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractText(tt.assistant)
			if got != tt.want {
				t.Errorf("extractText() = %q, want %q", got, tt.want)
			}
		})
	}
}
