package ccwire

import (
	"io"
	"strings"
	"testing"
)

// TestParser_ParseError_KnownTypes verifies that parse errors for known message types are returned.
func TestParser_ParseError_KnownTypes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errorSubstr string
	}{
		{
			name:        "corrupted_result_message",
			input:       `{"type":"result","subtype":"final","is_error":"not_a_bool"}`,
			expectError: true,
			errorSubstr: "failed to parse result message",
		},
		{
			name:        "corrupted_system_message",
			input:       `{"type":"system","subtype":"init","session_id":12345}`,
			expectError: true,
			errorSubstr: "failed to parse system message",
		},
		{
			name:        "corrupted_assistant_message",
			input:       `{"type":"assistant","message":"not_an_object","session_id":"s1"}`,
			expectError: true,
			errorSubstr: "failed to parse assistant message",
		},
		{
			name:        "corrupted_stream_event_message",
			input:       `{"type":"stream_event","event":"not_an_object","session_id":"s1"}`,
			expectError: true,
			errorSubstr: "failed to parse stream_event message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(strings.NewReader(tt.input))
			msg, err := parser.Next()

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errorSubstr)
				}
				if !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("expected error containing %q, got %q", tt.errorSubstr, err.Error())
				}
				if msg != nil {
					t.Errorf("expected nil message on error, got %T", msg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if msg == nil {
					t.Errorf("expected non-nil message")
				}
			}
		})
	}
}

// TestParser_MalformedJSON verifies that completely malformed JSON is skipped, not returned as an error.
func TestParser_MalformedJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "garbage_line",
			input: "not json at all\n",
		},
		{
			name:  "unclosed_brace",
			input: `{"type":"result"`,
		},
		{
			name:  "invalid_syntax",
			input: `{"type":}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(strings.NewReader(tt.input))
			msg, err := parser.Next()

			// Should skip malformed lines and return EOF
			if err != io.EOF {
				t.Errorf("expected io.EOF after skipping malformed line, got %v", err)
			}
			if msg != nil {
				t.Errorf("expected nil message, got %T", msg)
			}
		})
	}
}

// TestParser_UnknownMessageType verifies unknown message types are skipped.
func TestParser_UnknownMessageType(t *testing.T) {
	input := `{"type":"future_type","some_field":"value"}`
	parser := NewParser(strings.NewReader(input))
	msg, err := parser.Next()

	if err != io.EOF {
		t.Errorf("expected io.EOF after skipping unknown type, got %v", err)
	}
	if msg != nil {
		t.Errorf("expected nil message for unknown type, got %T", msg)
	}
}

// TestParser_ValidMessages verifies that valid messages are parsed correctly.
func TestParser_ValidMessages(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectType  MessageType
		expectError bool
	}{
		{
			name:       "valid_result_message",
			input:      `{"type":"result","subtype":"final","is_error":false,"result":"ok","duration_ms":100,"session_id":"s1","total_cost_usd":0.01,"usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0},"modelUsage":{}}`,
			expectType: TypeResult,
		},
		{
			name:       "valid_system_message",
			input:      `{"type":"system","subtype":"init","session_id":"s1","model":"claude-3","cwd":"/tmp","tools":["bash"]}`,
			expectType: TypeSystem,
		},
		{
			name:       "valid_assistant_message",
			input:      `{"type":"assistant","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-3","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}},"session_id":"s1"}`,
			expectType: TypeAssistant,
		},
		{
			name:       "valid_stream_event_message",
			input:      `{"type":"stream_event","event":{"type":"message_start"},"session_id":"s1"}`,
			expectType: TypeStreamEvent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(strings.NewReader(tt.input))
			msg, err := parser.Next()

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if msg == nil {
				t.Fatal("expected non-nil message")
			}
			if msg.MsgType() != tt.expectType {
				t.Errorf("expected type %s, got %s", tt.expectType, msg.MsgType())
			}
		})
	}
}

// TestParser_MixedLines verifies correct behavior with a mix of valid, malformed, and corrupted messages.
func TestParser_MixedLines(t *testing.T) {
	input := strings.Join([]string{
		``,                                          // Empty line (skipped)
		`not json`,                                  // Malformed JSON (skipped)
		`{"type":"future_type","data":"ignored"}`,   // Unknown type (skipped)
		`{"type":"system","subtype":"init","session_id":"s1","model":"claude-3","cwd":"/tmp","tools":[]}`, // Valid
		`{"type":"result","is_error":"bad"}`,        // Corrupted known type (error)
	}, "\n")

	parser := NewParser(strings.NewReader(input))

	// First Next() should return the valid system message
	msg1, err1 := parser.Next()
	if err1 != nil {
		t.Fatalf("expected valid system message, got error: %v", err1)
	}
	if msg1.MsgType() != TypeSystem {
		t.Errorf("expected system message, got %s", msg1.MsgType())
	}

	// Second Next() should return error for corrupted result message
	msg2, err2 := parser.Next()
	if err2 == nil {
		t.Fatal("expected error for corrupted result message, got nil")
	}
	if !strings.Contains(err2.Error(), "failed to parse result message") {
		t.Errorf("expected parse error, got: %v", err2)
	}
	if msg2 != nil {
		t.Errorf("expected nil message on error, got %T", msg2)
	}
}

// TestParser_CorruptedResultMessage_ErrorPropagation is the critical test case from issue #13.
// A corrupted ResultMessage should cause an error, not silently disappear.
func TestParser_CorruptedResultMessage_ErrorPropagation(t *testing.T) {
	// Simulate a corrupted result message where is_error field has wrong type
	input := `{"type":"result","subtype":"final","is_error":"true","result":"test","duration_ms":100,"session_id":"s1","total_cost_usd":0.0,"usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0},"modelUsage":{}}`

	parser := NewParser(strings.NewReader(input))
	msg, err := parser.Next()

	// Should return an error, not skip silently
	if err == nil {
		t.Fatal("expected parse error for corrupted ResultMessage, got nil")
	}

	// Error should mention the message type
	if !strings.Contains(err.Error(), "result") {
		t.Errorf("expected error to mention 'result' type, got: %v", err)
	}

	// Message should be nil
	if msg != nil {
		t.Errorf("expected nil message on parse error, got %T", msg)
	}
}

// TestParser_EmptyStream verifies EOF is returned for empty input.
func TestParser_EmptyStream(t *testing.T) {
	parser := NewParser(strings.NewReader(""))
	msg, err := parser.Next()

	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
	if msg != nil {
		t.Errorf("expected nil message, got %T", msg)
	}
}
