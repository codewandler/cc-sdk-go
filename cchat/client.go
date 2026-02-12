package cchat

import (
	"context"
	"fmt"
)

// Client manages Claude Code CLI subprocess interactions. It enforces an
// optional concurrency limit and applies default timeouts. A single Client
// should be reused for the lifetime of the application; it is safe for
// concurrent use by multiple goroutines.
type Client struct {
	cfg ClientConfig
	sem chan struct{} // concurrency semaphore; nil if unlimited
}

// NewClient creates a new [Client] with the given configuration. If
// cfg.CLIPath is empty it defaults to "claude". If cfg.MaxConcurrent is
// greater than zero, a buffered channel semaphore of that capacity is
// allocated to limit concurrent subprocess usage.
func NewClient(cfg *ClientConfig) *Client {
	c := &Client{
		cfg: *cfg,
	}
	if c.cfg.CLIPath == "" {
		c.cfg.CLIPath = "claude"
	}
	if cfg.MaxConcurrent > 0 {
		c.sem = make(chan struct{}, cfg.MaxConcurrent)
	}
	return c
}

// Query spawns a new claude CLI process with the given prompt and options,
// returning a [Stream] for reading the process output.
//
// The prompt is delivered to the subprocess via a stdin pipe to avoid OS
// argument length limits. If [ClientConfig].MaxConcurrent is set and all
// slots are occupied, Query blocks until a slot is freed or ctx is cancelled.
// If [ClientConfig].DefaultTimeout is set, a timeout-derived context is
// layered on top of ctx.
//
// The caller MUST call [Stream.Close] when done to kill the subprocess (if
// still running), reap the process, and release the concurrency semaphore
// slot. Failing to close the stream will leak resources.
func (c *Client) Query(ctx context.Context, prompt string, opts QueryOptions) (*Stream, error) {
	// Acquire semaphore slot
	if c.sem != nil {
		select {
		case c.sem <- struct{}{}:
		case <-ctx.Done():
			return nil, fmt.Errorf("acquiring semaphore: %w", ctx.Err())
		}
	}

	// Apply default timeout
	var timeoutCancel context.CancelFunc
	if c.cfg.DefaultTimeout > 0 {
		ctx, timeoutCancel = context.WithTimeout(ctx, c.cfg.DefaultTimeout)
	}

	proc, err := startProcess(ctx, c.cfg, opts, prompt)
	if err != nil {
		if timeoutCancel != nil {
			timeoutCancel()
		}
		c.releaseSem()
		return nil, err
	}

	// Store timeout cancel on process for cleanup in Stream.Close()
	proc.timeoutCancel = timeoutCancel

	return newStream(proc, c), nil
}

func (c *Client) releaseSem() {
	if c.sem != nil {
		<-c.sem
	}
}
