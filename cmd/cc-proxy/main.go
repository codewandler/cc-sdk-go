/*
Cc-proxy exposes the Claude Code CLI as an OpenAI-compatible HTTP inference
endpoint. Each incoming request spawns an isolated claude subprocess, translates
the OpenAI chat completion payload into a Claude Code prompt, and streams back
an OpenAI-format response. The process is fully stateless.

Usage:

	cc-proxy [flags]

Flags:

	-addr string
		Listen address for the HTTP server. (default ":8080")
	-model string
		Default Claude model to use (e.g. sonnet, opus, haiku).
		Can be overridden per-request via the model field in the request body.
	-api-key string
		Bearer token for authenticating incoming requests. When set, every
		request must include an "Authorization: Bearer <token>" header.
		If empty, authentication is disabled. Also read from the
		CC_PROXY_API_KEY environment variable when the flag is not provided.
	-claude-path string
		Path to the claude CLI binary. (default "claude")
	-max-concurrent int
		Maximum number of concurrent claude subprocesses. Zero means
		unlimited. (default 0)
	-timeout duration
		Per-request timeout applied to each claude subprocess. (default 5m)
	-work-dir string
		Working directory for spawned claude processes. If empty, the
		proxy's own working directory is used.

Environment variables:

	CC_PROXY_API_KEY
		Equivalent to -api-key. The flag takes precedence when both are set.

Endpoints:

	POST /v1/chat/completions   OpenAI-compatible chat completion (streaming and non-streaming)
	GET  /v1/models             Lists available models

The server performs a graceful shutdown on SIGINT or SIGTERM, allowing
in-flight requests to complete before exiting.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codewandler/cc-sdk-go/cchat"
	"github.com/codewandler/cc-sdk-go/server"
)

func main() {
	var (
		addr          = flag.String("addr", ":8080", "Listen address")
		model         = flag.String("model", "", "Default model (e.g. sonnet, opus)")
		apiKey        = flag.String("api-key", "", "API key for Bearer auth (empty = no auth)")
		claudePath    = flag.String("claude-path", "claude", "Path to claude binary")
		maxConcurrent = flag.Int("max-concurrent", 0, "Max concurrent claude processes (0 = unlimited)")
		timeout       = flag.Duration("timeout", 5*time.Minute, "Per-request timeout")
		workDir       = flag.String("work-dir", "", "Working directory for claude processes")
	)
	flag.Parse()

	// Allow API key from environment
	if *apiKey == "" {
		*apiKey = os.Getenv("CC_PROXY_API_KEY")
	}

	client := cchat.NewClient(&cchat.ClientConfig{
		CLIPath:        *claudePath,
		Model:          *model,
		MaxConcurrent:  *maxConcurrent,
		DefaultTimeout: *timeout,
		WorkDir:        *workDir,
	})

	srv := server.New(server.Config{
		Addr:   *addr,
		APIKey: *apiKey,
		Client: client,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stderr, "cc-proxy starting on %s\n", *addr)
	if *model != "" {
		fmt.Fprintf(os.Stderr, "default model: %s\n", *model)
	}
	if *apiKey != "" {
		fmt.Fprintln(os.Stderr, "auth: enabled")
	} else {
		fmt.Fprintln(os.Stderr, "auth: disabled")
	}
	if *maxConcurrent > 0 {
		fmt.Fprintf(os.Stderr, "max concurrent: %d\n", *maxConcurrent)
	}

	if err := srv.ListenAndServe(ctx); err != nil {
		log.Fatal(err)
	}
}
