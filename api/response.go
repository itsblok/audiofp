// Package api implements the HTTP API for the audio fingerprinting engine.
package api

import (
	"encoding/json"
	"net/http"
)

// envelope is the standard JSON response wrapper for every endpoint.
// Clients always parse the same top-level shape regardless of which endpoint
// they called. Error details live in Error; success payloads live in Data.
type envelope struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// writeJSON serialises resp to w with the given HTTP status code.
// It always sets Content-Type: application/json before writing the body,
// because headers cannot be set after WriteHeader is called.
func writeJSON(w http.ResponseWriter, status int, resp envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// Ignore encode error — if the connection is broken, nothing we can do.
	json.NewEncoder(w).Encode(resp)
}

// respondOK writes a 200 OK response with data as the payload.
func respondOK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: data})
}

// respondErr writes an error response with the given HTTP status and message.
func respondErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, envelope{OK: false, Error: msg})
}

// respondCreated writes a 201 Created response with data as the payload.
func respondCreated(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusCreated, envelope{OK: true, Data: data})
}
