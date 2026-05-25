// Package api — integration tests for all HTTP endpoints.
//
// These tests use a real Pipeline (full DSP chain) and a MemoryStore,
// exercised through httptest.NewRecorder so no TCP port is needed.
// All tests are self-contained: they synthesize their own WAV bytes.
package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/itsblok/audiofp/audio"
	"github.com/itsblok/audiofp/storage"
)

// --- Test helpers ---

// newTestServer builds a Server backed by a MemoryStore with a silent logger.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := storage.NewMemoryStore()
	pipeline := NewPipeline()
	logger := log.New(io.Discard, "", 0) // suppress log output during tests
	return NewServer(store, pipeline, logger)
}

// synthWAVBytes synthesizes a pure sine at freq Hz and encodes it as WAV bytes.
func synthWAVBytes(t *testing.T, freq float64, sampleRate int, durationSecs float64) []byte {
	t.Helper()
	n := int(float64(sampleRate) * durationSecs)
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
	}
	pcm := audio.PCM{Samples: samples, SampleRate: sampleRate}

	var buf bytes.Buffer
	if err := audio.EncodeWAV(&buf, pcm); err != nil {
		t.Fatalf("synthWAVBytes: %v", err)
	}
	return buf.Bytes()
}

// synthChordWAVBytes synthesizes a chord and encodes it as WAV bytes.
func synthChordWAVBytes(t *testing.T, freqs []float64, sampleRate int, durationSecs float64) []byte {
	t.Helper()
	n := int(float64(sampleRate) * durationSecs)
	samples := make([]float64, n)
	for i := range samples {
		tv := float64(i) / float64(sampleRate)
		var v float64
		for _, f := range freqs {
			v += math.Sin(2 * math.Pi * f * tv)
		}
		samples[i] = v / float64(len(freqs))
	}
	pcm := audio.PCM{Samples: samples, SampleRate: sampleRate}
	var buf bytes.Buffer
	audio.EncodeWAV(&buf, pcm)
	return buf.Bytes()
}

// makeMultipartRequest builds a multipart/form-data POST request carrying
// a WAV file in the "file" field and an optional "name" field.
func makeMultipartRequest(target, fieldName, filename, name string, wavBytes []byte) *http.Request {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	fw, _ := mw.CreateFormFile(fieldName, filename)
	fw.Write(wavBytes)

	if name != "" {
		mw.WriteField("name", name)
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// decodeResponse parses the JSON body of a recorded response.
func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
	return env
}

// --- Health endpoint ---

func TestHandleHealth_200(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	env := decodeResponse(t, rec)
	if !env.OK {
		t.Errorf("expected ok=true")
	}
}

// --- GET /songs ---

func TestHandleListSongs_EmptyStore(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/songs", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	env := decodeResponse(t, rec)
	if !env.OK {
		t.Errorf("expected ok=true")
	}
}

// --- POST /songs (index) ---

func TestHandleIndexSong_ValidWAV(t *testing.T) {
	srv := newTestServer(t)
	wavBytes := synthWAVBytes(t, 440.0, 44100, 5.0)
	req := makeMultipartRequest("/songs", "file", "A440.wav", "A440", wavBytes)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d (body: %s)", rec.Code, rec.Body)
	}
	env := decodeResponse(t, rec)
	if !env.OK {
		t.Errorf("expected ok=true, got error: %s", env.Error)
	}
}

func TestHandleIndexSong_NameFromFilename(t *testing.T) {
	// When no "name" field is provided, the song name should come from
	// the uploaded filename (without extension).
	srv := newTestServer(t)
	wavBytes := synthWAVBytes(t, 440.0, 44100, 3.0)

	// No name field — only the file.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "MySong.wav")
	fw.Write(wavBytes)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/songs", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestHandleIndexSong_DuplicateName(t *testing.T) {
	srv := newTestServer(t)
	wavBytes := synthWAVBytes(t, 440.0, 44100, 3.0)

	// Index once — should succeed.
	req1 := makeMultipartRequest("/songs", "file", "dup.wav", "DuplicateSong", wavBytes)
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first index: expected 201, got %d", rec1.Code)
	}

	// Index again with same name — should return 409 Conflict.
	req2 := makeMultipartRequest("/songs", "file", "dup.wav", "DuplicateSong", wavBytes)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Errorf("duplicate index: expected 409, got %d", rec2.Code)
	}
}

func TestHandleIndexSong_InvalidAudio(t *testing.T) {
	srv := newTestServer(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "garbage.wav")
	fw.Write([]byte("this is not a WAV file"))
	mw.WriteField("name", "Bad Song")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/songs", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid WAV, got %d", rec.Code)
	}
	env := decodeResponse(t, rec)
	if env.OK {
		t.Error("expected ok=false for invalid WAV")
	}
}

func TestHandleIndexSong_MissingFileField(t *testing.T) {
	srv := newTestServer(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("name", "No File") // name but no file
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/songs", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleIndexSong_WrongMethod(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/songs", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// --- POST /query ---

func TestHandleQuerySong_MatchFound(t *testing.T) {
	srv := newTestServer(t)

	// Index a chord.
	chordFreqs := []float64{440.0, 554.37, 659.25, 880.0}
	refWAV := synthChordWAVBytes(t, chordFreqs, 44100, 6.0)
	indexReq := makeMultipartRequest("/songs", "file", "amaj.wav", "A_major", refWAV)
	indexRec := httptest.NewRecorder()
	srv.ServeHTTP(indexRec, indexReq)
	if indexRec.Code != http.StatusCreated {
		t.Fatalf("index failed: %d %s", indexRec.Code, indexRec.Body)
	}

	// Query with the same audio — should match.
	queryReq := makeMultipartRequest("/query", "file", "query.wav", "", refWAV)
	queryRec := httptest.NewRecorder()
	srv.ServeHTTP(queryRec, queryReq)

	if queryRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on match, got %d (body: %s)", queryRec.Code, queryRec.Body)
	}
	env := decodeResponse(t, queryRec)
	if !env.OK {
		t.Errorf("expected ok=true: %s", env.Error)
	}
}

func TestHandleQuerySong_NoMatch(t *testing.T) {
	srv := newTestServer(t) // empty store

	wavBytes := synthWAVBytes(t, 440.0, 44100, 3.0)
	req := makeMultipartRequest("/query", "file", "query.wav", "", wavBytes)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 on no match, got %d", rec.Code)
	}
	env := decodeResponse(t, rec)
	if env.OK {
		t.Error("expected ok=false when no match")
	}
}

func TestHandleQuerySong_InvalidAudio(t *testing.T) {
	srv := newTestServer(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "bad.wav")
	fw.Write([]byte("not a wav"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/query", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleQuerySong_WrongMethod(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/query", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// --- Response format ---

func TestResponseHeaders(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct == "" {
		t.Error("expected Content-Type header")
	}
	rid := rec.Header().Get("X-Request-ID")
	if rid == "" {
		t.Error("expected X-Request-ID header from requestID middleware")
	}
}

// TestIndexThenQuery is the end-to-end integration test:
// index two distinct songs, then query each and verify correct identification.
func TestIndexThenQuery_CorrectSong(t *testing.T) {
	srv := newTestServer(t)

	songs := []struct {
		name  string
		freqs []float64
	}{
		{"A_major", []float64{440.0, 554.37, 659.25, 880.0}},
		{"D_major", []float64{293.66, 369.99, 440.0, 587.33}},
	}

	// Index both songs.
	for _, s := range songs {
		wavBytes := synthChordWAVBytes(t, s.freqs, 44100, 6.0)
		req := makeMultipartRequest("/songs", "file", s.name+".wav", s.name, wavBytes)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("index %s: %d", s.name, rec.Code)
		}
	}

	// Query each song and assert the correct one is returned.
	for _, s := range songs {
		wavBytes := synthChordWAVBytes(t, s.freqs, 44100, 6.0)
		queryReq := makeMultipartRequest("/query", "file", "q.wav", "", wavBytes)
		queryRec := httptest.NewRecorder()
		srv.ServeHTTP(queryRec, queryReq)

		if queryRec.Code != http.StatusOK {
			t.Errorf("query %s: expected 200, got %d", s.name, queryRec.Code)
			continue
		}

		// Parse the matched song name from the JSON data field.
		var resp struct {
			OK   bool `json:"ok"`
			Data struct {
				Name string `json:"name"`
			} `json:"data"`
		}
		json.NewDecoder(queryRec.Body).Decode(&resp)
		if resp.Data.Name != s.name {
			t.Errorf("query %s: matched %q instead", s.name, resp.Data.Name)
		}
	}
}
