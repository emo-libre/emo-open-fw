package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// statusRecorder remembers the status code for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// logged wraps a handler with a one-line access log.
//
// Deliberately logs only method, path and status. Headers (Authorization, Secret) and bodies are never logged
// or stored, and the query string is dropped because it can carry location data.
func logged(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d", r.Method, strings.TrimSpace(r.URL.Path), rec.status)
	})
}
