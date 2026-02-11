package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codewandler/cc-sdk-go/cchat"
	"github.com/codewandler/cc-sdk-go/ccwire"
	"github.com/codewandler/cc-sdk-go/oai"
)

// mockStream implements StreamReader for testing without spawning real claude processes.
type mockStream struct {
	messages []ccwire.Message
	index    int
}

func (m *mockStream) Next() (ccwire.Message, error) {
	if m.index >= len(m.messages) {
		return nil, io.EOF
	}
	msg := m.messages[m.index]
	m.index++
	return msg, nil
}

func (m *mockStream) Close() error {
	return nil
}

// mockClient implements the cchat.Client interface for testing.
type mockClient struct {
	stream *mockStream
}

func (m *mockClient) Query(ctx context.Context, prompt string, opts cchat.QueryOptions) (*cchat.Stream, error) {
	// Return a mock stream - type assertion will fail but we'll use our own StreamReader interface
	return nil, nil
}

// TestMaxBytesReader verifies that oversized request bodies are rejected.
func TestMaxBytesReader(t *testing.T) {
	tests := []struct {
		name           string
		bodySize       int
		expectError    bool
		errorSubstring string
	}{
		{
			name:        "small_request_succeeds",
			bodySize:    1024, // 1KB
			expectError: false,
		},
		{
			name:        "moderate_request_succeeds",
			bodySize:    1 << 20, // 1MB
			expectError: false,
		},
		{
			name:           "oversized_request_fails",
			bodySize:       11 << 20, // 11MB (exceeds 10MB limit)
			expectError:    true,
			errorSubstring: "Invalid JSON",
		},
		{
			name:        "near_limit_succeeds",
			bodySize:    (10 << 20) - 1024, // Just under 10MB
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple test handler that just checks MaxBytesReader behavior
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req oai.ChatCompletionRequest
				r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB limit
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON: "+err.Error())
					return
				}

				// If we got here, the request was valid
				// For testing purposes, just return a success response
				if !tt.expectError {
					resp := oai.ChatCompletionResponse{
						ID:    "test-id",
						Model: "test-model",
						Choices: []oai.Choice{
							{
								Index:   0,
								Message: oai.ChatMessage{Role: "assistant", Content: "test"},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
				}
			})

			// Create a valid request body of the specified size
			reqBody := createRequestBody(tt.bodySize)

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if tt.expectError {
				if w.Code == http.StatusOK {
					t.Errorf("expected error response, got status %d", w.Code)
				}
				if tt.errorSubstring != "" && !strings.Contains(w.Body.String(), tt.errorSubstring) {
					t.Errorf("expected error message to contain %q, got: %s", tt.errorSubstring, w.Body.String())
				}
			} else {
				if w.Code != http.StatusOK {
					t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

// TestMaxBytesReaderIntegration tests the actual server handler with MaxBytesReader.
func TestMaxBytesReaderIntegration(t *testing.T) {
	// Create server with real handler
	srv := New(Config{
		Addr:   ":8080",
		Client: &cchat.Client{},
	})

	// Test 1: Small valid request should work (will fail at client.Query, but MaxBytesReader should pass)
	t.Run("small_request_passes_maxbytes", func(t *testing.T) {
		smallReq := oai.ChatCompletionRequest{
			Model:    "test",
			Messages: []oai.ChatMessage{{Role: "user", Content: "hello"}},
		}
		body, _ := json.Marshal(smallReq)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleChatCompletions(w, req)

		// Should not fail with MaxBytesReader error (will fail later at Query, but that's expected)
		if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "http: request body too large") {
			t.Error("MaxBytesReader rejected a valid small request")
		}
	})

	// Test 2: Oversized request should be rejected by MaxBytesReader
	t.Run("oversized_request_rejected", func(t *testing.T) {
		// Create a 11MB payload
		largeBody := createRequestBody(11 << 20)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.handleChatCompletions(w, req)

		// Should get a bad request error
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}

		// The error should mention it's a JSON/body issue
		bodyStr := w.Body.String()
		if !strings.Contains(bodyStr, "Invalid JSON") {
			t.Errorf("expected error message about invalid JSON, got: %s", bodyStr)
		}
	})
}

// createRequestBody generates a valid JSON request body of approximately the specified size.
func createRequestBody(targetSize int) []byte {
	// Start with a minimal valid request
	baseReq := oai.ChatCompletionRequest{
		Model:    "test-model",
		Messages: []oai.ChatMessage{{Role: "user", Content: ""}},
	}

	// Calculate how much padding we need
	baseJSON, _ := json.Marshal(baseReq)
	baseSize := len(baseJSON)

	if targetSize <= baseSize {
		return baseJSON
	}

	// Add padding to the content field
	paddingSize := targetSize - baseSize + 10 // +10 for JSON overhead
	padding := strings.Repeat("x", paddingSize)

	paddedReq := oai.ChatCompletionRequest{
		Model:    "test-model",
		Messages: []oai.ChatMessage{{Role: "user", Content: padding}},
	}

	result, _ := json.Marshal(paddedReq)
	return result
}
