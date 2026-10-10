// Package handlers declares handlers in another package than the routes, to test
// cross-package resolution.
package handlers

import (
	"domain"
	"fake"
	"httpres"
)

// Keeps the httpres import: the //effectgo:handler directives name it.
var _ = httpres.ToHTTPError

// Update is a handler constructor, with its errmap.
//
//effectgo:handler httpres.ToHTTPError
func Update(svc int) func(fake.Ctx[string, string]) (int, error) {
	return func(c fake.Ctx[string, string]) (int, error) {
		if svc == 0 {
			return 0, domain.ErrNotFound
		}
		return 0, domain.ErrConflict
	}
}

// Show is a handler function, with its errmap.
//
//effectgo:handler httpres.ToHTTPError
func Show(c fake.Ctx[string, string]) (int, error) {
	return 0, domain.ErrNotFound
}
