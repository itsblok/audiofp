// Package api — HTTP server with route table and graceful shutdown.
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/itsblok/audiofp/storage"
)

// Server is the HTTP server for the fingerprinting engine.
// It owns the route table, middleware chain, and lifecycle management.
type Server struct {
	store    storage.Store
	pipeline *Pipeline
	logger   *log.Logger
	handler  http.Handler // fully composed middleware + routes
}

// NewServer constructs a Server and wires all routes and middleware.
func NewServer(store storage.Store, pipeline *Pipeline, logger *log.Logger) *Server {
	s := &Server{
		store:    store,
		pipeline: pipeline,
		logger:   logger,
	}
	s.handler = chain(
		s.routes(),
		withRequestID(),
		withLogger(logger),
		withRecovery(logger),
		withMaxBodySize(maxUploadBytes),
	)
	return s
}

// routes registers all API endpoints on a fresh ServeMux.
//
// Route table:
//   GET  /health   — liveness probe
//   GET  /songs    — list indexed songs + store stats
//   POST /songs    — index a new WAV file
//   POST /query    — identify an unknown WAV clip
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/songs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleListSongs(w, r)
		case http.MethodPost:
			s.handleIndexSong(w, r)
		default:
			respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	mux.HandleFunc("/query", s.handleQuerySong)

	return mux
}

// ServeHTTP implements http.Handler, making Server directly usable in tests
// via httptest.NewRecorder without starting a real TCP listener.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// Serve starts the HTTP server on addr and blocks until ctx is cancelled.
// On cancellation, it initiates a graceful shutdown with a 10-second drain:
// in-flight requests complete, new connections are refused.
func (s *Server) Serve(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second, // generous for large WAV uploads
		IdleTimeout:  120 * time.Second,
	}

	// Run the listener in a goroutine; report startup errors via errCh.
	errCh := make(chan error, 1)
	go func() {
		s.logger.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Block until context cancellation or a fatal startup error.
	select {
	case <-ctx.Done():
		s.logger.Printf("shutting down (drain 10s)...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		s.logger.Printf("shutdown complete")
		return nil

	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}
