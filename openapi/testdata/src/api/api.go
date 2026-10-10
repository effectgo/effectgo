// Package api registers the routes that the openapi tests check.
package api

import (
	"domain"
	"fake"
	"handlers"
	"httpres"
)

// Keeps the httpres import: the //effectgo:handler directives name it.
var _ = httpres.ToHTTPError

// update is a handler constructor with the errmap ToHTTPError.
//
//effectgo:handler httpres.ToHTTPError
func update(svc int) func(fake.Ctx[string, string]) (int, error) { // want update:"."
	return func(c fake.Ctx[string, string]) (int, error) {
		if svc == 0 {
			return 0, domain.ErrNotFound
		}
		return 0, domain.ErrConflict
	}
}

// list is a handler function; it returns a plain error, which the errmap leaves
// unmapped (500).
//
//effectgo:handler httpres.ToHTTPError
func list(c fake.Ctx[string, string]) (int, error) { // want list:"."
	return 0, domain.ValidationErr{Field: "x"}
}

// bare is a constructor with no errmap directive.
func bare(svc int) func(fake.Ctx[string, string]) (int, error) { // want bare:"."
	return func(c fake.Ctx[string, string]) (int, error) { return 0, domain.ErrNotFound }
}

// wrap returns its argument: the handler is not visible through it.
func wrap(h func(fake.Ctx[string, string]) (int, error)) func(fake.Ctx[string, string]) (int, error) { // want wrap:"."
	return h
}

// Register registers the test routes.
func Register(s *fake.Server, svc int, p string) { // want Register:"."
	fake.Put(s, "/items/{id}", update(svc))
	fake.Get(s, "/items", list)
	fake.Get(s, "/lit", func(c fake.Ctx[string, string]) (int, error) { return 0, domain.ErrNotFound })
	fake.Post(s, "/wrapped", wrap(update(svc)))
	fake.Delete(s, p, update(svc))
	fake.Put(s, "/nodirective", bare(svc))
	fake.Put(s, "/cross/{id}", handlers.Update(svc))
	fake.Get(s, "/show", handlers.Show)
}
