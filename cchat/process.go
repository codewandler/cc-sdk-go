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

// ProcessError is returned when the CC process exits with a non-zero code.
type ProcessError struct {
	ExitCode int
	Stderr   string
}

func (e *ProcessError) Error() string {
	return fmt.Sprintf("claude process exited with code %d: %s", e.ExitCode, e.Stderr)
}
