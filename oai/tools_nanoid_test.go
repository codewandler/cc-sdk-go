package oai

import (
	"strings"
	"testing"
)

// TestToolCallIDFormat verifies that tool call IDs use the nanoid format.
func TestToolCallIDFormat(t *testing.T) {
	input := `<tool_call>{"name": "test1", "arguments": {}}</tool_call><tool_call>{"name": "test2", "arguments": {}}</tool_call>`
	_, calls := ParseToolCalls(input)

	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	seenIDs := make(map[string]bool)
	for i, call := range calls {
		// Check format: call_<nanoid>
		if !strings.HasPrefix(call.ID, "call_") {
			t.Errorf("call[%d].ID = %q, want format call_<nanoid>", i, call.ID)
		}

		// Check that the suffix is not empty
		suffix := strings.TrimPrefix(call.ID, "call_")
		if suffix == "" {
			t.Errorf("call[%d].ID = %q, nanoid suffix is empty", i, call.ID)
		}

		// Check for uniqueness
		if seenIDs[call.ID] {
			t.Errorf("call[%d].ID = %q, duplicate ID found", i, call.ID)
		}
		seenIDs[call.ID] = true
	}

	// Verify IDs are globally unique (not sequential)
	if calls[0].ID == calls[1].ID {
		t.Errorf("IDs should be unique, but got duplicate: %q", calls[0].ID)
	}

	// Verify IDs are not the old sequential format
	if calls[0].ID == "call_0" || calls[1].ID == "call_1" {
		t.Errorf("IDs should use nanoid, but got sequential format: %q, %q", calls[0].ID, calls[1].ID)
	}
}
