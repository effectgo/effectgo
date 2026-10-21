package main

import (
	"log/slog"
	"net/http"

	"github.com/effectgo/effectgo/rt/web"
)

// NewHandler routes the two endpoints of the spike onto store through rt/web.
// The framework decodes the input (a bad path or body is a DecodeError, 400),
// encodes the errors by their StatusCode, and turns any other error into a
// 500 without its text. The middleware are the boundary concerns that the
// novastar port writes by hand: request ID, access log and a body limit.
func NewHandler(store *Store, logger *slog.Logger) http.Handler {
	r := web.New(
		web.WithLogger(logger),
		web.WithMiddleware(
			web.RequestID(),
			web.AccessLog(logger),
			web.BodyLimit(1<<20),
		),
	)
	web.Get(r, "/api/contacts/{id}", store.GetContact)
	web.Post(r, "/api/contact", store.UpdateContact)
	return r
}
