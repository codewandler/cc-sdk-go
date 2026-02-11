package cchat

import (
	"context"
	"fmt"
)

// Client manages Claude Code CLI subprocess interactions.
type Client struct {
	cfg ClientConfig
	sem chan struct{} // concurrency semaphore; nil if unlimited
}

// NewClient creates a new Client with the given configuration.
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

// Query starts a Claude Code process with the given prompt and options.
// The prompt is delivered via stdin pipe. Returns a Stream for reading results.
// The caller must call Stream.Close() when done.
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
	if c.cfg.DefaultTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.DefaultTimeout)
		// cancel will be called in Stream.Close()
		_ = cancel
	}

	proc, err := startProcess(ctx, c.cfg, opts, prompt)
	if err != nil {
		c.releaseSem()
		return nil, err
	}

	return newStream(proc, c), nil
}

func (c *Client) releaseSem() {
	if c.sem != nil {
		<-c.sem
	}
}
