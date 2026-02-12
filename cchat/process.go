package cchat

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// processInterface defines the minimal interface for process operations
// needed by Stream. This allows tests to use mock implementations.
type processInterface interface {
	wait() error
	kill()
	getStdout() io.ReadCloser
	getStderr() *bytes.Buffer
}

// process wraps an exec.Cmd for a Claude Code CLI subprocess.
type process struct {
	cmd           *exec.Cmd
	stdout        io.ReadCloser
	stderr        *bytes.Buffer
	cancel        context.CancelFunc
	timeoutCancel context.CancelFunc // cancel for timeout context, if any
}

// startProcess spawns a claude CLI process with the given configuration.
// The prompt is written to stdin then stdin is closed.
func startProcess(ctx context.Context, cfg ClientConfig, opts QueryOptions, prompt string) (*process, error) {
	ctx, cancel := context.WithCancel(ctx)

	args := buildArgs(cfg, opts)

	cmd := exec.CommandContext(ctx, cfg.CLIPath, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}

	// Set up stdin pipe for prompt delivery
	cmd.Stdin = strings.NewReader(prompt)

	// Capture stdout for NDJSON parsing
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	// Capture stderr for error reporting
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting claude process: %w", err)
	}

	return &process{
		cmd:    cmd,
		stdout: stdout,
		stderr: &stderr,
		cancel: cancel,
	}, nil
}

func buildArgs(cfg ClientConfig, opts QueryOptions) []string {
	args := []string{
		"--print",
		"--output-format=stream-json",
		"--verbose",
		"--tools=",
		"--disable-slash-commands",
		"--no-session-persistence",
		"--setting-sources=",
		"--strict-mcp-config",
	}

	model := opts.Model
	if model == "" {
		model = cfg.Model
	}
	if model != "" {
		args = append(args, "--model="+model)
	}

	// Always pass --system-prompt to replace the ~3k token default.
	// An empty value gives the model no system prompt at all.
	args = append(args, "--system-prompt="+opts.SystemPrompt)

	if opts.Streaming {
		args = append(args, "--include-partial-messages")
	}

	if opts.Effort != "" {
		args = append(args, "--effort="+opts.Effort)
	}

	return args
}

// wait waits for the process to exit and returns any error.
func (p *process) wait() error {
	return p.cmd.Wait()
}

// kill terminates the process and cleans up all context resources.
func (p *process) kill() {
	p.cancel()
	if p.timeoutCancel != nil {
		p.timeoutCancel()
	}
}

// getStdout returns the stdout reader for parsing process output.
func (p *process) getStdout() io.ReadCloser {
	return p.stdout
}

// getStderr returns the stderr buffer for error reporting.
func (p *process) getStderr() *bytes.Buffer {
	return p.stderr
}

// ProcessError is returned by [Stream.Next] or [Stream.Result] when the
// claude CLI process exits with a non-zero exit code. It wraps the exit
// code and any output written to stderr, which typically contains
// human-readable error messages from the CLI.
//
// Callers can use a type assertion or [errors.As] to inspect the error:
//
//	var procErr *cchat.ProcessError
//	if errors.As(err, &procErr) {
//		log.Printf("exit %d: %s", procErr.ExitCode, procErr.Stderr)
//	}
type ProcessError struct {
	// ExitCode is the non-zero exit code returned by the claude process.
	ExitCode int

	// Stderr contains the full contents of the process's standard error
	// stream at the time the error was captured.
	Stderr string
}

// Error returns a human-readable representation of the process failure,
// including the exit code and stderr output.
func (e *ProcessError) Error() string {
	return fmt.Sprintf("claude process exited with code %d: %s", e.ExitCode, e.Stderr)
}

// RateLimitError is returned by [Stream.Next] when the Claude Code CLI
// reports a rate limit exceeded error. This typically occurs when the user
// has exceeded their API quota. The error message contains details about
// when the limit will reset.
//
// Callers can use a type assertion or [errors.As] to inspect the error:
//
//	var rateErr *cchat.RateLimitError
//	if errors.As(err, &rateErr) {
//		http.Error(w, rateErr.Message, http.StatusTooManyRequests)
//	}
type RateLimitError struct {
	// Message contains the human-readable rate limit error message,
	// typically including when the limit will reset.
	Message string
}

// Error returns the rate limit error message.
func (e *RateLimitError) Error() string {
	return e.Message
}
