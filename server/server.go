package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/codewandler/cc-sdk-go/cchat"
	"github.com/codewandler/cc-sdk-go/ccwire"
)

// StreamReader is the interface consumed by the server to read messages from a
// Claude Code subprocess stream. It matches the signature of [cchat.Stream] but
// is defined as an interface so handlers can be tested with mock implementations
// that don't spawn real processes.
//
// Next returns the next parsed message or [io.EOF] when the stream is exhausted.
// Close releases any resources held by the underlying stream.
type StreamReader interface {
	Next() (ccwire.Message, error)
	Close() error
}

// Config holds the settings used to create a [Server].
type Config struct {
	// Addr is the TCP address for the server to listen on, in the form "host:port".
	// If empty, the server listens on all interfaces with a system-chosen port.
	Addr string

	// APIKey is the expected Bearer token for authenticating inbound requests.
	// When non-empty, every request must include an "Authorization: Bearer <key>"
	// header whose value matches this key (compared in constant time). When empty,
	// the auth middleware is bypassed entirely and all requests are allowed through.
	APIKey string

	// Client is the cchat.Client used to spawn Claude Code subprocesses.
	// It must be non-nil.
	Client *cchat.Client
}

// Server is an OpenAI-compatible HTTP server that translates chat completion
// requests into Claude Code CLI subprocess calls and returns the results in
// OpenAI format. Use [New] to create an instance and [Server.ListenAndServe]
// to start serving.
type Server struct {
	cfg    Config
	client *cchat.Client
	mux    *http.ServeMux
}

// New creates a [Server] with the given configuration and registers the
// /v1/chat/completions and /v1/models routes. The returned server is ready
// to be started with [Server.ListenAndServe] or used directly via
// [Server.Handler] for custom HTTP serving arrangements.
func New(cfg Config) *Server {
	s := &Server{
		cfg:    cfg,
		client: cfg.Client,
		mux:    http.NewServeMux(),
	}

	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/v1/models", s.handleModels)

	return s
}

// Handler returns the fully assembled [http.Handler] with the middleware stack
// applied (panic recovery, request logging, and optional Bearer token auth).
// This is useful for testing or for mounting the server inside a custom
// [http.Server].
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = authMiddleware(s.cfg.APIKey, h)
	h = loggingMiddleware(h)
	h = recoveryMiddleware(h)
	return h
}

// ListenAndServe starts the HTTP server on the address specified in [Config.Addr]
// and blocks until ctx is cancelled or the server fails to start.
//
// When ctx is cancelled, the server initiates a graceful shutdown with a
// 15-second deadline, allowing in-flight requests (including active SSE streams)
// to complete before forcibly closing connections. If the server shuts down
// cleanly within the deadline, ListenAndServe returns nil.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.cfg.Addr,
		Handler: s.Handler(),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", s.cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Println("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
