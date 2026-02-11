// Package server provides an OpenAI-compatible HTTP server backed by Claude Code.
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
