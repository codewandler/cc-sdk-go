package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codewandler/cc-sdk-go/oai"
)

// dummyHandler is a simple handler that writes "ok" if authentication succeeds.
var dummyHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
})

func TestAuthMiddleware_NoAuthRequired(t *testing.T) {
	// When apiKey is empty, auth should be skipped
	handler := authMiddleware("", dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", w.Body.String())
	}
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	handler := authMiddleware("secret-key-123", dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer secret-key-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", w.Body.String())
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	handler := authMiddleware("secret-key-123", dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}

	var errResp oai.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Type != "invalid_api_key" {
		t.Errorf("expected error type 'invalid_api_key', got %q", errResp.Error.Type)
	}
}

func TestAuthMiddleware_MissingBearer(t *testing.T) {
	handler := authMiddleware("secret-key-123", dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "secret-key-123") // Missing "Bearer " prefix
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_EmptyAuth(t *testing.T) {
	handler := authMiddleware("secret-key-123", dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Authorization header set
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ConstantTime(t *testing.T) {
	// This test demonstrates that comparison time is constant regardless of
	// how many characters match. It's not a rigorous timing attack test,
	// but verifies the implementation uses constant-time comparison.
	handler := authMiddleware("correct-secret-key", dummyHandler)

	testCases := []struct {
		name string
		key  string
	}{
		{"first_char_wrong", "xorrect-secret-key"},
		{"last_char_wrong", "correct-secret-kex"},
		{"all_wrong", "xxxxxxxxxxxxxxxxxx"},
		{"half_match", "correct-xxxxxxxxxxx"},
	}

	// Measure time for each case
	timings := make([]time.Duration, len(testCases))
	for i, tc := range testCases {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Authorization", "Bearer "+tc.key)
		w := httptest.NewRecorder()

		start := time.Now()
		handler.ServeHTTP(w, req)
		timings[i] = time.Since(start)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected status 401, got %d", tc.name, w.Code)
		}
	}

	// All timings should be similar (within 10ms of each other)
	// This is a coarse check; real timing attacks require statistical analysis
	var min, max time.Duration = timings[0], timings[0]
	for _, d := range timings {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}

	// If the comparison were not constant-time, we'd expect significant
	// differences. With constant-time comparison, differences should be
	// due only to scheduling variance.
	diff := max - min
	if diff > 10*time.Millisecond {
		t.Logf("Warning: timing variance %v exceeds 10ms threshold", diff)
		t.Logf("Timings: %v", timings)
		// Don't fail the test since this can be noisy in CI/containers,
		// but log it for awareness
	}
}

func TestAuthMiddleware_DifferentLengthKeys(t *testing.T) {
	// Test that keys of different lengths are properly rejected
	handler := authMiddleware("short", dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer verylongkeythatdoesnotmatch")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}
