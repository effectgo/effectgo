// Package web is the HTTP adapter of the example. Its statusWriter embeds the
// response writer and defines no Write of its own, so Write is promoted.
package web

import (
	"net/http"

	"example.com/statuswriter/diag"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

// NewWriter returns the response as a diag.Writer.
func NewWriter(w http.ResponseWriter) diag.Writer {
	return &statusWriter{ResponseWriter: w}
}
