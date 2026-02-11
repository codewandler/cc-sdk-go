package oai

import (
	"context"
	"io"

	"github.com/codewandler/cc-sdk-go/cchat"
	"github.com/codewandler/cc-sdk-go/ccwire"
)

// ChatCompletionStream reads streaming chat completion chunks from Claude Code.
type ChatCompletionStream struct {
	raw           *cchat.Stream
	state         *StreamState
	lastAssistant *ccwire.AssistantMessage
	pending       []*ChatCompletionChunk
	err           error
}

// CreateChatCompletionStream sends a streaming chat completion request.
func (c *Client) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionStream, error) {
	req.Stream = true
	prompt, opts := RequestToQuery(&req)

	stream, err := c.cc.Query(ctx, prompt, opts)
	if err != nil {
		return nil, &APIError{Message: err.Error(), Type: "service_unavailable"}
	}

	return &ChatCompletionStream{
		raw:   stream,
		state: NewStreamState(len(req.Tools) > 0),
	}, nil
}

// Recv returns the next streaming chunk. Returns io.EOF when the stream is done.
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

// Close terminates the stream and releases resources.
func (cs *ChatCompletionStream) Close() error {
	cs.err = io.EOF
	return cs.raw.Close()
}
