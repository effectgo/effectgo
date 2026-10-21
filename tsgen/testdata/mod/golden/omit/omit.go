// Package omit is a golden case: omitempty on scalars, pointers, slices and
// maps in a response, and nil slices and maps without omitempty (NullOr). The
// route has no input (web.NoInput), so it has no DecodeError.
package omit

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

// OmitOut is the response of GET /api/omit.
type OmitOut struct {
	S  string         `json:"s,omitempty"`
	I  int            `json:"i,omitempty"`
	P  *string        `json:"p,omitempty"`
	L  []string       `json:"l,omitempty"`
	M  map[string]int `json:"m,omitempty"`
	N  *int           `json:"n"`
	Ls []string       `json:"ls"`
}

// Omit returns a value that leaves the omitted fields at their zero value.
func Omit(_ context.Context, _ web.NoInput) (OmitOut, error) {
	return OmitOut{S: "x"}, nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Get(r, "/api/omit", Omit)
}
