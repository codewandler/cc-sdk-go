package cchat

import (
	"io"
	"os/exec"
	"sync"

	"github.com/codewandler/cc-sdk-go/ccwire"
)

// Stream reads typed [ccwire.Message] values from a running Claude Code
// process. Messages are parsed incrementally from the process's stdout
// NDJSON output via a [ccwire.Parser].
//
// A Stream holds two resources that must be released: the underlying
// subprocess and a concurrency semaphore slot on the parent [Client].
// Callers MUST call [Stream.Close] when finished, typically via defer.
// Close is idempotent and safe to call multiple times.
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

// Next reads and returns the next [ccwire.Message] from the stream.
//
// When all messages have been consumed, Next waits for the subprocess to
// exit. If the process exits cleanly, Next returns (nil, [io.EOF]). If
// the process exits with a non-zero code, Next returns a [*ProcessError]
// containing the exit code and stderr contents. If a rate limit error
// is detected in an AssistantMessage, Next returns a [*RateLimitError].
// Subsequent calls to Next after EOF return (nil, [io.EOF]) immediately.
//
// The concrete message types returned are [*ccwire.SystemMessage],
// [*ccwire.AssistantMessage], [*ccwire.ResultMessage], and
// [*ccwire.StreamEventMessage]. The last [*ccwire.ResultMessage] seen is
// cached and available via [Stream.Result].
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

	// Check for rate limit error in AssistantMessage
	if am, ok := msg.(*ccwire.AssistantMessage); ok && am.Error == "rate_limit" {
		// Extract error message from content blocks
		var errorMsg string
		for _, block := range am.Message.Content {
			if block.Type == "text" {
				errorMsg = block.Text
				break
			}
		}
		if errorMsg == "" {
			errorMsg = "rate limit exceeded"
		}
		return nil, &RateLimitError{Message: errorMsg}
	}

	// Cache result message
	if rm, ok := msg.(*ccwire.ResultMessage); ok {
		s.result = rm
	}

	return msg, nil
}

// Result is a convenience method that drains the stream by calling [Next]
// repeatedly until [io.EOF], then returns the final [*ccwire.ResultMessage].
// All intermediate messages are discarded.
//
// If the stream ends without a ResultMessage (e.g., the process was
// killed), Result returns [io.ErrUnexpectedEOF]. Any error from [Next]
// (including [*ProcessError]) is propagated as-is.
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

// Close terminates the stream and releases all associated resources. If
// the subprocess is still running, it is killed and reaped to prevent
// zombie processes. The concurrency semaphore slot on the parent [Client]
// is always released, regardless of whether the stream was fully consumed.
//
// Close is idempotent: multiple calls are safe and always return nil.
// It should be called exactly once per stream, typically via defer
// immediately after [Client.Query].
func (s *Stream) Close() error {
	s.closeOnce.Do(func() {
		if !s.done {
			s.proc.kill()
			s.proc.wait() // Reap the process to prevent zombies
			s.done = true
		}
		s.client.releaseSem()
	})
	return nil
}
