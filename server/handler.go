package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/codewandler/cc-sdk-go/ccwire"
	"github.com/codewandler/cc-sdk-go/oai"
)

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is accepted")
		return
	}

	var req oai.ChatCompletionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB limit
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Messages array is required")
		return
	}

	prompt, opts := oai.RequestToQuery(&req)

	stream, err := s.client.Query(r.Context(), prompt, opts)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "Failed to start claude process: "+err.Error())
		return
	}
	defer stream.Close()

	if req.Stream {
		s.handleStreamingResponse(w, stream, len(req.Tools) > 0)
	} else {
		s.handleNonStreamingResponse(w, stream, len(req.Tools) > 0)
	}
}

func (s *Server) handleStreamingResponse(w http.ResponseWriter, stream StreamReader, hasTools bool) {
	sse := newSSEWriter(w)
	state := oai.NewStreamState(hasTools)
	var lastAssistant *ccwire.AssistantMessage

	for {
		msg, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("stream error: %v", err)
			break
		}

		switch m := msg.(type) {
		case *ccwire.StreamEventMessage:
			chunks := state.HandleStreamEvent(m)
			for _, chunk := range chunks {
				if err := sse.WriteEvent(chunk); err != nil {
					return
				}
			}

		case *ccwire.AssistantMessage:
			lastAssistant = m

		case *ccwire.ResultMessage:
			// Emit finish chunks
			finishChunks := state.FinishChunk(lastAssistant)
			for _, chunk := range finishChunks {
				if err := sse.WriteEvent(chunk); err != nil {
					return
				}
			}

			if m.IsError {
				log.Printf("claude error: %s", m.Result)
			}
		}
	}

	sse.WriteDone()
}

func (s *Server) handleNonStreamingResponse(w http.ResponseWriter, stream StreamReader, hasTools bool) {
	var lastAssistant *ccwire.AssistantMessage
	var result *ccwire.ResultMessage

	for {
		msg, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Stream error: "+err.Error())
			return
		}

		switch m := msg.(type) {
		case *ccwire.AssistantMessage:
			lastAssistant = m
		case *ccwire.ResultMessage:
			result = m
		}
	}

	if result == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "No result received from claude")
		return
	}

	if result.IsError {
		status := http.StatusInternalServerError
		writeError(w, status, "claude_error", result.Result)
		return
	}

	resp := oai.ResultToResponse(result, lastAssistant, hasTools)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is accepted")
		return
	}

	models := []map[string]any{
		{"id": "sonnet", "object": "model", "owned_by": "anthropic"},
		{"id": "opus", "object": "model", "owned_by": "anthropic"},
		{"id": "haiku", "object": "model", "owned_by": "anthropic"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   models,
	})
}

func writeError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(oai.ErrorResponse{
		Error: oai.ErrorDetail{
			Message: message,
			Type:    errType,
		},
	})
}
