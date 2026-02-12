package server

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/codewandler/cc-sdk-go/cchat"
)

// TestListenAndServe_GracefulShutdown verifies that the server shuts down gracefully when context is cancelled.
func TestListenAndServe_GracefulShutdown(t *testing.T) {
	// Use a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	client := cchat.NewClient(&cchat.ClientConfig{
		CLIPath: "claude",
		Model:   "sonnet",
	})

	srv := New(Config{
		Addr:   addr,
		APIKey: "",
		Client: client,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()

	// Wait for server to be ready
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if i == maxRetries-1 {
			t.Fatalf("server did not become ready after %d retries", maxRetries)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Cancel context to trigger shutdown
	cancel()

	// Verify shutdown completes within reasonable time (well under the 15s deadline)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected error during shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not complete within 2 seconds")
	}
}

// TestListenAndServe_ShutdownDeadline verifies that the shutdown deadline is enforced.
func TestListenAndServe_ShutdownDeadline(t *testing.T) {
	// Use a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	client := cchat.NewClient(&cchat.ClientConfig{
		CLIPath: "claude",
		Model:   "sonnet",
	})

	srv := New(Config{
		Addr:   addr,
		APIKey: "",
		Client: client,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()

	// Wait for server to be ready
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if i == maxRetries-1 {
			t.Fatalf("server did not become ready after %d retries", maxRetries)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Start a long-running request that won't complete
	go func() {
		client := &http.Client{Timeout: 30 * time.Second}
		req, _ := http.NewRequest("GET", "http://"+addr+"/v1/models", nil)
		client.Do(req) // This will be interrupted by shutdown
	}()

	// Give the request time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	shutdownStart := time.Now()
	cancel()

	// Verify shutdown completes within the 15s deadline plus some margin
	select {
	case err := <-errCh:
		shutdownDuration := time.Since(shutdownStart)
		if err != nil && err != http.ErrServerClosed && err != context.DeadlineExceeded {
			t.Errorf("unexpected error during shutdown: %v", err)
		}
		// Shutdown should complete within 15s + 1s margin (but likely much faster)
		if shutdownDuration > 16*time.Second {
			t.Errorf("shutdown took %v, expected <= 16s", shutdownDuration)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("shutdown did not complete within 20 seconds (deadline is 15s)")
	}
}
