// Package rtname declares types whose names are the names rt/web and rt/schema
// write in the TypeScript file. Each one collides with a framework name.
package rtname

import (
	"context"
	"net/http"

	"github.com/faintghost/effectgo/rt/web"
)

// In is the input of POST /api/rtname.
type In struct {
	Q    string `query:"q"`
	Body string `json:"body"`
}

// Out is the response; Issue is named like rt/schema's Issue.
type Out struct {
	Issues []Issue `json:"issues"`
}

// Issue is a user type named like rt/schema.Issue.
type Issue struct {
	Path string `json:"path"`
}

// InternalError is a user error named like rt/web.InternalError.
type InternalError struct{}

// Error describes the error.
func (InternalError) Error() string { return "internal" }

// StatusCode reports 500.
func (InternalError) StatusCode() int { return http.StatusInternalServerError }

// DecodeError is a user error named like rt/schema.DecodeError.
type DecodeError struct{}

// Error describes the error.
func (DecodeError) Error() string { return "decode" }

// StatusCode reports 400.
func (DecodeError) StatusCode() int { return http.StatusBadRequest }

// BodyTooLarge is a user error named like rt/web.BodyTooLarge.
type BodyTooLarge struct{}

// Error describes the error.
func (BodyTooLarge) Error() string { return "large" }

// StatusCode reports 413.
func (BodyTooLarge) StatusCode() int { return http.StatusRequestEntityTooLarge }

// Save returns one of the user errors.
func Save(_ context.Context, in In) (Out, error) {
	switch in.Q {
	case "a":
		return Out{}, InternalError{}
	case "b":
		return Out{}, DecodeError{}
	}
	return Out{}, BodyTooLarge{}
}

// Register adds the route.
func Register(r *web.Router) {
	web.Post(r, "/api/rtname", Save)
}
