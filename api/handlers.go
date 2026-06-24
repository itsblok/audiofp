// Package api — HTTP request handlers.
//
// Each handler has one job: parse the request, call the pipeline or store,
// and write a JSON response. No DSP logic lives here.
//
// Upload convention: audio files are sent as multipart/form-data with
// field name "file". This is curl-compatible:
//   curl -F file=@song.wav http://localhost:8080/songs
//   curl -F file=@clip.wav http://localhost:8080/query
package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cheemney/audiofp/audio"
	"github.com/cheemney/audiofp/storage"
)

const (
	// maxUploadBytes caps the WAV upload size. A 3-minute 44.1kHz 16-bit
	// mono WAV is ~15 MB; 50 MB gives comfortable headroom for stereo/longer.
	maxUploadBytes = 50 << 20 // 50 MB
)

// handleHealth returns a liveness signal. Used by load balancers and monitors.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondOK(w, map[string]string{"status": "ok", "version": "step5"})
}

// handleListSongs returns all registered songs and store statistics.
//
// GET /songs
func (s *Server) handleListSongs(w http.ResponseWriter, r *http.Request) {
	stats := s.store.Stats()
	respondOK(w, map[string]interface{}{
		"stats": map[string]int{
			"songs":          stats.Songs,
			"hash_entries":   stats.HashEntries,
			"total_postings": stats.TotalPostings,
		},
	})
}

// handleIndexSong fingerprints an uploaded WAV and stores it in the database.
//
// POST /songs
// Content-Type: multipart/form-data
// Fields:
//   file — the WAV file (required)
//   name — the song name (optional; defaults to the uploaded filename)
func (s *Server) handleIndexSong(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse the multipart form. The 32 MB in-memory limit means files up to
	// 32 MB are held in RAM; larger ones spill to a temp file automatically.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondErr(w, http.StatusBadRequest,
			"failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, "missing 'file' field in form")
		return
	}
	defer file.Close()

	// Song name: use the form field if provided, otherwise strip the extension
	// from the uploaded filename.
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}
	if name == "" {
		name = "unknown"
	}

	result, err := s.pipeline.Index(file, name, s.store)
	if err != nil {
		// Distinguish between client errors (bad WAV) and server errors.
		if isWAVError(err) {
			respondErr(w, http.StatusBadRequest, "invalid audio: "+err.Error())
			return
		}
		if errors.Is(err, storage.ErrDuplicateSong) {
			respondErr(w, http.StatusConflict,
				"song '"+name+"' is already indexed")
			return
		}
		s.logger.Printf("index error for %q: %v", name, err)
		respondErr(w, http.StatusInternalServerError, "indexing failed")
		return
	}

	respondCreated(w, result)
}

// handleQuerySong fingerprints an uploaded WAV clip and returns the best match.
//
// POST /query
// Content-Type: multipart/form-data
// Fields:
//   file — the WAV file to identify (required)
//
// Returns 200 + match data on success, 404 when no match is found.
func (s *Server) handleQuerySong(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondErr(w, http.StatusBadRequest,
			"failed to parse multipart form: "+err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, "missing 'file' field in form")
		return
	}
	defer file.Close()

	result, ok, err := s.pipeline.Query(file, s.store)
	if err != nil {
		if isWAVError(err) {
			respondErr(w, http.StatusBadRequest, "invalid audio: "+err.Error())
			return
		}
		s.logger.Printf("query error: %v", err)
		respondErr(w, http.StatusInternalServerError, "query failed")
		return
	}

	if !ok {
		respondErr(w, http.StatusNotFound, "no match found")
		return
	}

	respondOK(w, result)
}

// isWAVError reports whether err originates from the WAV decoder,
// indicating a malformed or unsupported client upload (a 4xx, not a 5xx).
func isWAVError(err error) bool {
	return errors.Is(err, audio.ErrNotWAV) ||
		errors.Is(err, audio.ErrNoFmtChunk) ||
		errors.Is(err, audio.ErrNoDataChunk) ||
		errors.Is(err, audio.ErrUnsupportedFormat) ||
		errors.Is(err, audio.ErrUnsupportedDepth) ||
		errors.Is(err, audio.ErrEmptyAudio)
}
