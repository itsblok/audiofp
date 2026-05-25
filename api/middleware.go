// Package api — HTTP middleware chain.
//
// Middlewares are composed as: requestID → logger → recovery → handler.
// Each wraps the next, so execution order is outside-in for setup,
// and inside-out for teardown (defer/recover).
package api

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// middleware is a function that wraps an http.Handler to add behaviour.
type middleware func(http.Handler) http.Handler

// chain composes middlewares left-to-right around a base handler.
// chain(h, A, B, C) executes as A(B(C(h))): A's setup runs first,
// C's setup runs last (closest to the actual handler).
func chain(h http.Handler, middlewares ...middleware) http.Handler {
	// Apply in reverse so the first middleware in the list is outermost.
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// statusRecorder wraps http.ResponseWriter to capture the status code
// after it has been written, for use in access logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Write captures an implicit 200 if WriteHeader was never called.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// withLogger logs each request's method, path, status, and latency.
// Format: 2006/01/02 15:04:05 GET /query 200 42ms
func withLogger(logger *log.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			logger.Printf("%-6s %-20s %d  %s",
				r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
		})
	}
}

// withRecovery catches panics in handlers, logs them, and returns 500.
// Without this, a panic in one handler would bring down the entire server.
func withRecovery(logger *log.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Printf("PANIC: %v", rec)
					respondErr(w, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// withRequestID stamps every response with a monotonically increasing
// X-Request-ID header. Useful for correlating client and server logs.
func withRequestID() middleware {
	var counter uint64
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter++
			w.Header().Set("X-Request-ID", fmt.Sprintf("%d", counter))
			next.ServeHTTP(w, r)
		})
	}
}

// withMaxBodySize limits the request body to prevent OOM on large uploads.
// Returns 413 Request Entity Too Large if the limit is exceeded.
func withMaxBodySize(maxBytes int64) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
