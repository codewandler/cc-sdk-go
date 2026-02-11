// Package cchat provides a Go SDK for interacting with the Claude Code CLI.
package cchat

import "time"

// ClientConfig configures a cchat Client.
type ClientConfig struct {
	// CLIPath is the path to the claude binary. Default: "claude".
	CLIPath string

	// Model is the default model to use (--model flag).
	Model string

	// MaxConcurrent is the maximum number of concurrent CC processes.
	// 0 means unlimited.
	MaxConcurrent int

	// DefaultTimeout is the per-process timeout. 0 means context-only.
	DefaultTimeout time.Duration

	// WorkDir is the working directory for CC processes.
	WorkDir string
}

// QueryOptions configures a single query.
type QueryOptions struct {
	// SystemPrompt replaces CC's default system prompt via --system-prompt.
	SystemPrompt string

	// Streaming adds --include-partial-messages for streaming output.
	Streaming bool

	// Model overrides the client's default model for this query.
	Model string

	// Effort sets the --effort flag (low/medium/high).
	Effort string
}
