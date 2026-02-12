// Package server provides an OpenAI-compatible HTTP server backed by Claude Code
// CLI subprocesses.
//
// The server exposes two endpoints:
//
//   - POST /v1/chat/completions — Accepts OpenAI-format chat completion requests,
//     translates them into Claude Code subprocess calls via the [oai] bridge, and
//     returns responses in OpenAI format. Both streaming (Server-Sent Events) and
//     non-streaming modes are supported.
//   - GET /v1/models — Returns the list of available Claude models.
//
// Inbound requests pass through a middleware stack applied in the following order:
//
//  1. Panic recovery — catches panics and returns a 500 JSON error.
//  2. Logging — logs method, path, status code, and duration for every request.
//  3. Auth — validates Bearer tokens using constant-time comparison. Skipped when
//     no API key is configured.
//
// # Usage
//
//	cfg := server.Config{
//		Addr:   ":8080",
//		APIKey: "sk-my-secret", // optional; leave empty to disable auth
//		Client: chatClient,     // *cchat.Client
//	}
//	srv := server.New(cfg)
//	if err := srv.ListenAndServe(ctx); err != nil {
//		log.Fatal(err)
//	}
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// sseWriter wraps an http.ResponseWriter for Server-Sent Events.
type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func newSSEWriter(w http.ResponseWriter) *sseWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	return &sseWriter{w: w, flusher: flusher}
}

// WriteEvent writes a single SSE event with the given data.
func (s *sseWriter) WriteEvent(data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.w, "data: %s\n\n", jsonData)
	if err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

// WriteDone writes the final [DONE] event.
func (s *sseWriter) WriteDone() {
	fmt.Fprint(s.w, "data: [DONE]\n\n")
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

// WriteError writes an SSE error event with the appropriate HTTP status code.
// This is used for unrecoverable errors that occur during streaming.
func (s *sseWriter) WriteError(status int, errType, message string) {
	s.w.WriteHeader(status)
	jsonData, _ := json.Marshal(map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    errType,
		},
	})
	fmt.Fprintf(s.w, "data: %s\n\n", jsonData)
	if s.flusher != nil {
		s.flusher.Flush()
	}
}
