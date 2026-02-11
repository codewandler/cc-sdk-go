// cc-proxy exposes Claude Code as an OpenAI-compatible HTTP inference endpoint.
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
