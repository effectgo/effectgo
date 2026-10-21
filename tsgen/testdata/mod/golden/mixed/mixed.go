// Package mixed is a golden case: an input with a path field, a query field and
// body fields. The payload is inline, and the path and query keys are the URL
// names (id, page), not the json names.
package mixed

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

// MixedIn is the input of PUT /api/mixed/{id}?page=N.
type MixedIn struct {
	ID    int    `path:"id" json:"id" schema:"min=1"`
	Page  int    `query:"page" json:"pageNo" schema:"min=1"`
	Name  string `json:"name" schema:"trim,nonempty"`
	Email string `json:"email" schema:"email"`
}

// MixedOut is the response of PUT /api/mixed/{id}.
type MixedOut struct {
	Saved bool `json:"saved"`
}

// Update reports whether the item was saved.
func Update(_ context.Context, in MixedIn) (MixedOut, error) {
	return MixedOut{Saved: in.ID > 0 && in.Page > 0}, nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Put(r, "/api/mixed/{id}", Update)
}
