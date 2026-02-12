package oai

import (
	"context"
	"io"

	"github.com/codewandler/cc-sdk-go/cchat"
	"github.com/codewandler/cc-sdk-go/ccwire"
)

// ChatCompletionStream provides an iterator-style interface for reading
// streaming chat completion chunks from a Claude Code process. Obtain one by
// calling [Client.CreateChatCompletionStream]. Call [ChatCompletionStream.Recv]
// repeatedly to receive chunks until [io.EOF] is returned. The stream must be
// closed with [ChatCompletionStream.Close] when no longer needed.
//
// Internally, ChatCompletionStream reads [ccwire] messages from the underlying
// [cchat.Stream], translates them through [StreamState], and queues the
// resulting [ChatCompletionChunk] values for delivery.
type ChatCompletionStream struct {
	raw           *cchat.Stream
	state         *StreamState
	lastAssistant *ccwire.AssistantMessage
	pending       []*ChatCompletionChunk
	err           error
}

// CreateChatCompletionStream sends a streaming chat completion request to the
// Claude Code CLI and returns a [ChatCompletionStream] for reading incremental
// chunks. The request's Stream field is forced to true regardless of its input
// value.
//
// It returns an [*APIError] on failure, with the same error types as
// [Client.CreateChatCompletion]. The caller must call [ChatCompletionStream.Close]
// when finished reading to terminate the underlying claude process.
func (c *Client) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionStream, error) {
	if err := c.Effort.validate(); err != nil {
		return nil, &APIError{Message: err.Error(), Type: "invalid_request_error"}
	}
	req.Stream = true
	prompt, opts := RequestToQuery(&req)
	opts.Effort = string(c.Effort)

	stream, err := c.cc.Query(ctx, prompt, opts)
	if err != nil {
		return nil, &APIError{Message: err.Error(), Type: "service_unavailable"}
	}

	return &ChatCompletionStream{
		raw:   stream,
		state: NewStreamState(len(req.Tools) > 0),
	}, nil
}

// Recv returns the next [ChatCompletionChunk] from the stream. It blocks until
// a chunk is available, an error occurs, or the stream ends. Returns [io.EOF]
// when the stream is complete.
//
// After an error (including io.EOF), all subsequent calls return the same error.
// Chunks may be queued internally when a single Claude Code event produces
// multiple OAI chunks (e.g. remaining text plus tool calls at stream finish).
func (cs *ChatCompletionStream) Recv() (*ChatCompletionChunk, error) {
	// Sticky error
	if cs.err != nil {
		return nil, cs.err
	}

	// Drain pending queue first
	if len(cs.pending) > 0 {
		chunk := cs.pending[0]
		cs.pending = cs.pending[1:]
		return chunk, nil
	}

	// Read from cchat stream until we have chunks to emit
	for {
		msg, err := cs.raw.Next()
		if err == io.EOF {
			cs.err = io.EOF
			return nil, io.EOF
		}
		if err != nil {
			cs.err = err
			return nil, err
		}

		switch m := msg.(type) {
		case *ccwire.StreamEventMessage:
			chunks := cs.state.HandleStreamEvent(m)
			if len(chunks) > 0 {
				cs.pending = append(cs.pending, chunks[1:]...)
				return chunks[0], nil
			}

		case *ccwire.AssistantMessage:
			cs.lastAssistant = m

		case *ccwire.ResultMessage:
			finishChunks := cs.state.FinishChunk(cs.lastAssistant)
			if len(finishChunks) > 0 {
				cs.pending = append(cs.pending, finishChunks[1:]...)
				return finishChunks[0], nil
			}
		}
	}
}

// Close terminates the streaming response and releases resources, including
// killing the underlying claude CLI process. After Close, any pending or
// future calls to [ChatCompletionStream.Recv] return [io.EOF].
func (cs *ChatCompletionStream) Close() error {
	cs.err = io.EOF
	return cs.raw.Close()
}
