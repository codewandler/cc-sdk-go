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

// StreamReader is the interface for reading CC stream messages.
// Matches *cchat.Stream but allows testing with mock implementations.
type StreamReader interface {
	Next() (ccwire.Message, error)
	Close() error
}

// Config configures the HTTP server.
type Config struct {
	Addr   string // Listen address, e.g. ":8080"
	APIKey string // Expected Bearer token; empty means no auth
	Client *cchat.Client
}

// Server is an OpenAI-compatible HTTP server backed by Claude Code.
type Server struct {
	cfg    Config
	client *cchat.Client
	mux    *http.ServeMux
}

// New creates a new Server.
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

// Handler returns the http.Handler with middleware applied.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = authMiddleware(s.cfg.APIKey, h)
	h = loggingMiddleware(h)
	h = recoveryMiddleware(h)
	return h
}

// ListenAndServe starts the HTTP server and blocks until ctx is cancelled.
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
