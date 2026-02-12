package oai

import (
	"encoding/json"
	"testing"
)

func TestParseToolCalls(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantText      string
		wantCallCount int
		wantCalls     []ToolCall
	}{
		{
			name:          "no_tool_calls",
			input:         "Just a regular response with no tool calls.",
			wantText:      "Just a regular response with no tool calls.",
			wantCallCount: 0,
		},
		{
			name:     "single_tool_call",
			input:    `<tool_call>{"name": "get_weather", "arguments": {"city": "Paris"}}</tool_call>`,
			wantText: "",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Paris"}`,
					},
				},
			},
		},
		{
			name:     "multiple_tool_calls",
			input:    `<tool_call>{"name": "get_weather", "arguments": {"city": "Tokyo"}}</tool_call><tool_call>{"name": "get_weather", "arguments": {"city": "London"}}</tool_call>`,
			wantText: "",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Tokyo"}`,
					},
				},
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"London"}`,
					},
				},
			},
		},
		{
			name:     "text_before_tool_call",
			input:    `Here is the weather info: <tool_call>{"name": "get_weather", "arguments": {"city": "Berlin"}}</tool_call>`,
			wantText: "Here is the weather info:",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Berlin"}`,
					},
				},
			},
		},
		{
			name:     "text_after_tool_call",
			input:    `<tool_call>{"name": "send_email", "arguments": {"to": "test@example.com"}}</tool_call> Email will be sent shortly.`,
			wantText: "Email will be sent shortly.",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "send_email",
						Arguments: `{"to":"test@example.com"}`,
					},
				},
			},
		},
		{
			name:     "text_between_multiple_calls",
			input:    `First: <tool_call>{"name": "tool_a", "arguments": {}}</tool_call> and second: <tool_call>{"name": "tool_b", "arguments": {}}</tool_call> done.`,
			wantText: "First:  and second:  done.",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "tool_a",
						Arguments: `{}`,
					},
				},
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "tool_b",
						Arguments: `{}`,
					},
				},
			},
		},
		{
			name:          "malformed_json_missing_quote",
			input:         `<tool_call>{"name": "bad_tool, "arguments": {}}</tool_call>`,
			wantText:      `<tool_call>{"name": "bad_tool, "arguments": {}}</tool_call>`,
			wantCallCount: 0,
		},
		{
			name:          "malformed_json_trailing_comma",
			input:         `<tool_call>{"name": "bad_tool", "arguments": {},}</tool_call>`,
			wantText:      `<tool_call>{"name": "bad_tool", "arguments": {},}</tool_call>`,
			wantCallCount: 0,
		},
		{
			name:          "malformed_json_invalid_structure",
			input:         `<tool_call>not valid json at all</tool_call>`,
			wantText:      `<tool_call>not valid json at all</tool_call>`,
			wantCallCount: 0,
		},
		{
			name:          "unclosed_tag",
			input:         `<tool_call>{"name": "incomplete", "arguments": {}}`,
			wantText:      `<tool_call>{"name": "incomplete", "arguments": {}}`,
			wantCallCount: 0,
		},
		{
			name:          "unopened_tag",
			input:         `{"name": "no_opening", "arguments": {}}</tool_call>`,
			wantText:      `{"name": "no_opening", "arguments": {}}</tool_call>`,
			wantCallCount: 0,
		},
		{
			name:     "nested_xml_like_content",
			input:    `<tool_call>{"name": "test", "arguments": {"data": "<foo>bar</foo>"}}</tool_call>`,
			wantText: "",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "test",
						Arguments: `{"data":"<foo>bar</foo>"}`,
					},
				},
			},
		},
		{
			name:          "non_object_arguments_string",
			input:         `<tool_call>{"name": "test", "arguments": "string_not_object"}</tool_call>`,
			wantText:      `<tool_call>{"name": "test", "arguments": "string_not_object"}</tool_call>`,
			wantCallCount: 0,
		},
		{
			name:          "non_object_arguments_array",
			input:         `<tool_call>{"name": "test", "arguments": ["array", "not", "object"]}</tool_call>`,
			wantText:      `<tool_call>{"name": "test", "arguments": ["array", "not", "object"]}</tool_call>`,
			wantCallCount: 0,
		},
		{
			name:     "non_object_arguments_null",
			input:    `<tool_call>{"name": "test", "arguments": null}</tool_call>`,
			wantText: "",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "test",
						Arguments: `null`,
					},
				},
			},
		},
		{
			name:          "non_object_arguments_number",
			input:         `<tool_call>{"name": "test", "arguments": 123}</tool_call>`,
			wantText:      `<tool_call>{"name": "test", "arguments": 123}</tool_call>`,
			wantCallCount: 0,
		},
		{
			name:          "empty_tag",
			input:         `<tool_call></tool_call>`,
			wantText:      `<tool_call></tool_call>`,
			wantCallCount: 0,
		},
		{
			name:          "whitespace_only_tag",
			input:         `<tool_call>   </tool_call>`,
			wantText:      `<tool_call>   </tool_call>`,
			wantCallCount: 0,
		},
		{
			name:     "missing_arguments_field",
			input:    `<tool_call>{"name": "test"}</tool_call>`,
			wantText: "",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "test",
						Arguments: `null`,
					},
				},
			},
		},
		{
			name:     "missing_name_field",
			input:    `<tool_call>{"arguments": {"key": "value"}}</tool_call>`,
			wantText: "",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "",
						Arguments: `{"key":"value"}`,
					},
				},
			},
		},
		{
			name:     "empty_arguments_object",
			input:    `<tool_call>{"name": "no_args", "arguments": {}}</tool_call>`,
			wantText: "",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "no_args",
						Arguments: `{}`,
					},
				},
			},
		},
		{
			name:     "complex_nested_arguments",
			input:    `<tool_call>{"name": "complex", "arguments": {"user": {"name": "John", "age": 30}, "tags": ["a", "b"]}}</tool_call>`,
			wantText: "",
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "complex",
						Arguments: `{"tags":["a","b"],"user":{"age":30,"name":"John"}}`,
					},
				},
			},
		},
		{
			name:     "mixed_valid_and_invalid",
			input:    `<tool_call>{"name": "valid", "arguments": {}}</tool_call><tool_call>invalid json</tool_call><tool_call>{"name": "also_valid", "arguments": {}}</tool_call>`,
			wantText: `<tool_call>invalid json</tool_call>`,
			wantCalls: []ToolCall{
				{
					ID:   "call_0",
					Type: "function",
					Function: FunctionCall{
						Name:      "valid",
						Arguments: `{}`,
					},
				},
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "also_valid",
						Arguments: `{}`,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotCalls := ParseToolCalls(tt.input)

			if gotText != tt.wantText {
				t.Errorf("ParseToolCalls() text = %q, want %q", gotText, tt.wantText)
			}

			if tt.wantCalls != nil {
				if len(gotCalls) != len(tt.wantCalls) {
					t.Fatalf("ParseToolCalls() call count = %d, want %d", len(gotCalls), len(tt.wantCalls))
				}

				for i, wantCall := range tt.wantCalls {
					gotCall := gotCalls[i]
					if gotCall.ID != wantCall.ID {
						t.Errorf("call[%d].ID = %q, want %q", i, gotCall.ID, wantCall.ID)
					}
					if gotCall.Type != wantCall.Type {
						t.Errorf("call[%d].Type = %q, want %q", i, gotCall.Type, wantCall.Type)
					}
					if gotCall.Function.Name != wantCall.Function.Name {
						t.Errorf("call[%d].Function.Name = %q, want %q", i, gotCall.Function.Name, wantCall.Function.Name)
					}

					var gotArgs, wantArgs map[string]any
					if err := json.Unmarshal([]byte(gotCall.Function.Arguments), &gotArgs); err != nil {
						t.Errorf("call[%d].Function.Arguments invalid JSON: %v", i, err)
					}
					if err := json.Unmarshal([]byte(wantCall.Function.Arguments), &wantArgs); err != nil {
						t.Fatalf("test case invalid: wantCall.Arguments not valid JSON: %v", err)
					}

					gotJSON, _ := json.Marshal(gotArgs)
					wantJSON, _ := json.Marshal(wantArgs)
					if string(gotJSON) != string(wantJSON) {
						t.Errorf("call[%d].Function.Arguments = %s, want %s", i, gotJSON, wantJSON)
					}
				}
			} else if tt.wantCallCount >= 0 {
				if len(gotCalls) != tt.wantCallCount {
					t.Errorf("ParseToolCalls() call count = %d, want %d", len(gotCalls), tt.wantCallCount)
				}
			}
		})
	}
}

func TestHasToolCallPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "no_tag",
			input: "Just regular text",
			want:  false,
		},
		{
			name:  "complete_tag",
			input: `<tool_call>{"name": "test", "arguments": {}}</tool_call>`,
			want:  true,
		},
		{
			name:  "partial_tag_opening",
			input: "Some text <tool_ca",
			want:  false,
		},
		{
			name:  "partial_tag_complete_opening",
			input: "Some text <tool_call",
			want:  true,
		},
		{
			name:  "partial_tag_with_content",
			input: `<tool_call>{"name": "test"`,
			want:  true,
		},
		{
			name:  "only_opening_bracket",
			input: "Some text <",
			want:  false,
		},
		{
			name:  "closing_tag_only",
			input: "Some text </tool_call>",
			want:  false,
		},
		{
			name:  "tag_in_middle",
			input: "Before <tool_call> after",
			want:  true,
		},
		{
			name:  "empty_string",
			input: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasToolCallPrefix(tt.input); got != tt.want {
				t.Errorf("HasToolCallPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}
