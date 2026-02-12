package cchat

import (
	"io"
	"os/exec"
	"sync"

	"github.com/codewandler/cc-sdk-go/ccwire"
)

// Stream reads typed messages from a running Claude Code process.
type Stream struct {
	proc      processInterface
	parser    *ccwire.Parser
	client    *Client
	done      bool
	result    *ccwire.ResultMessage
	closeOnce sync.Once
}

func newStream(proc *process, client *Client) *Stream {
	return &Stream{
		proc:   proc,
		parser: ccwire.NewParser(proc.getStdout()),
		client: client,
	}
}

// Next reads and returns the next message from the stream.
// Returns io.EOF when the stream is exhausted.
func (s *Stream) Next() (ccwire.Message, error) {
	if s.done {
		return nil, io.EOF
	}

	msg, err := s.parser.Next()
	if err == io.EOF {
		s.done = true
		// Wait for the process to finish
		if waitErr := s.proc.wait(); waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				return nil, &ProcessError{
					ExitCode: exitErr.ExitCode(),
					Stderr:   s.proc.getStderr().String(),
				}
			}
			// Surface non-ExitError wait failures (e.g., I/O errors)
			return nil, waitErr
		}
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}

	// Cache result message
	if rm, ok := msg.(*ccwire.ResultMessage); ok {
		s.result = rm
	}

	return msg, nil
}

// Result blocks until the stream is fully consumed and returns the ResultMessage.
func (s *Stream) Result() (*ccwire.ResultMessage, error) {
	for {
		_, err := s.Next()
		if err == io.EOF {
			if s.result != nil {
				return s.result, nil
			}
			return nil, io.ErrUnexpectedEOF
		}
		if err != nil {
			return nil, err
		}
	}
}

// Close terminates the stream and releases resources.
// Multiple calls are safe (idempotent).
func (s *Stream) Close() error {
	s.closeOnce.Do(func() {
		if !s.done {
			s.proc.kill()
			s.done = true
		}
		s.client.releaseSem()
	})
	return nil
}
