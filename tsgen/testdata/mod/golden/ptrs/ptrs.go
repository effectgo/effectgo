// Package ptrs is a golden case: a pointer field in the request and in the
// response, with and without omitempty.
package ptrs

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

// PtrIn is the request body of PUT /api/ptrs.
type PtrIn struct {
	Name *string `json:"name"`
	Note *string `json:"note,omitempty"`
	Age  *int    `json:"age"`
}

// PtrOut is the response of PUT /api/ptrs.
type PtrOut struct {
	Name *string `json:"name"`
	Note *string `json:"note,omitempty"`
	Age  *int    `json:"age"`
}

// Pointers echoes the input.
func Pointers(_ context.Context, in PtrIn) (PtrOut, error) {
	return PtrOut(in), nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Put(r, "/api/ptrs", Pointers)
}
