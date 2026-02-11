package server

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
	"time"
)

// authMiddleware validates Bearer token authentication.
func authMiddleware(apiKey string, next http.Handler) http.Handler {
	if apiKey == "" {
		return next // No auth required
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
			return
		}
		providedKey := strings.TrimPrefix(auth, "Bearer ")
		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(apiKey)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap for http.Flusher support
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// recoveryMiddleware catches panics and returns 500.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %v", err)
				writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
