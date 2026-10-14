// Package rtapi registers routes the way examples/web does: generic route
// functions of the rt/web shape, with method values as handlers.
package rtapi

import (
	"context"

	"domain"
	"httpres"
	"rtfake"
)

// Keeps the httpres import: the //effectgo:handler directives name it.
var _ = httpres.ToHTTPError

type Store struct{}

type Input struct{ ID int }

type Contact struct{}

// Get returns domain.ErrNotFound, which the errmap maps to 404.
//
//effectgo:handler httpres.ToHTTPError
func (s *Store) Get(ctx context.Context, in Input) (Contact, error) { // want Get:"."
	return Contact{}, domain.ErrNotFound
}

// Update returns a conflict (409) or a plain error (500).
//
//effectgo:handler httpres.ToHTTPError
func (s *Store) Update(ctx context.Context, in Input) (Contact, error) { // want Update:"."
	if in.ID == 0 {
		return Contact{}, domain.ErrConflict
	}
	return Contact{}, domain.ValidationErr{Field: "id"}
}

// Register passes the handlers as method values.
func Register(r *rtfake.Router, s *Store) { // want Register:"."
	rtfake.Get(r, "/contacts/{id}", s.Get)
	rtfake.Post(r, "/contact", s.Update)
}
