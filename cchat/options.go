// Package cchat wraps the Claude Code CLI as a subprocess, providing a
// streaming message reader for Claude's NDJSON output. It sits between the
// oai bridge layer (which translates OpenAI requests) and the ccwire parser
// (which decodes Claude's wire-format messages).
//
// Concurrency is managed with a buffered channel semaphore: when
// [ClientConfig].MaxConcurrent is set, at most that many claude processes may
// run simultaneously. Additional calls to [Client.Query] block until a slot
// is available or the context is cancelled.
//
// Prompts are delivered to the claude process via stdin pipe rather than
// command-line arguments to avoid OS argument length limits.
//
// Basic usage:
//
//	client := cchat.NewClient(&cchat.ClientConfig{
//		Model:         "sonnet",
//		MaxConcurrent: 4,
//	})
//
//	stream, err := client.Query(ctx, "Explain concurrency in Go", cchat.QueryOptions{
//		SystemPrompt: "You are a helpful assistant.",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer stream.Close()
//
//	for {
//		msg, err := stream.Next()
//		if err == io.EOF {
//			break
//		}
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Println(msg)
//	}
package cchat

import "time"

// ClientConfig holds the configuration for a [Client]. All fields are
// optional and have sensible zero-value defaults.
type ClientConfig struct {
	// CLIPath is the filesystem path to the claude binary.
	// If empty, "claude" is used (resolved via PATH).
	CLIPath string

	// Model is the default model identifier passed as the --model flag
	// to the claude CLI. It can be overridden per-query via
	// [QueryOptions].Model.
	Model string

	// MaxConcurrent limits the number of claude processes that may run
	// simultaneously. When the limit is reached, [Client.Query] blocks
	// until a slot is freed or the context is cancelled. A value of 0
	// (the default) means unlimited concurrency.
	MaxConcurrent int

	// DefaultTimeout applies a per-process deadline to every query.
	// The timeout starts when [Client.Query] spawns the subprocess.
	// A value of 0 (the default) means no timeout is applied beyond
	// the caller-supplied context.
	DefaultTimeout time.Duration

	// WorkDir sets the working directory for spawned claude processes.
	// If empty, the processes inherit the parent's working directory.
	WorkDir string
}

// QueryOptions configures a single [Client.Query] invocation. All fields
// are optional.
type QueryOptions struct {
	// SystemPrompt replaces Claude Code's default system prompt via the
	// --system-prompt flag. An empty string is passed through, giving
	// the model no system prompt at all (the ~3k token default is
	// always replaced for determinism and better cache hit rates).
	SystemPrompt string

	// Streaming enables partial message output by adding the
	// --include-partial-messages flag. When true, the [Stream] will
	// yield intermediate [ccwire.AssistantMessage] values in addition
	// to the final [ccwire.ResultMessage].
	Streaming bool

	// Model overrides [ClientConfig].Model for this query. If empty,
	// the client's default model is used.
	Model string

	// Effort sets the --effort flag, which controls the model's
	// thinking effort level. Valid values are "low", "medium", and
	// "high". If empty, the flag is omitted and the CLI default
	// applies.
	Effort string
}
