// Package web is the HTTP adapter of the example. Its statusWriter embeds the
// response writer and defines its own Write, which writes to standard I/O only.
package web

import (
	"net/http"

	"example.com/ownwriter/diag"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

// Write is the type's own method: it writes to the standard output.
func (s *statusWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

// NewWriter returns the response as a diag.Writer.
func NewWriter(w http.ResponseWriter) diag.Writer {
	return &statusWriter{ResponseWriter: w}
}
