// Package opaque holds the routes that the unclassified-error tests check: the
// "?" variant is 500 unless the errmap maps it, and a dynamic status stays
// unknown (default).
package opaque

import (
	"errors"
	"fmt"

	"domain"
	"fake"
)

// unclassifiedFormat is not a constant, so an error formatted with it is opaque
// ("?"). An error created with a constant format is fresh instead ("new@").
var unclassifiedFormat = "unclassified %v"

// ConflictError has a constant status: 409.
type ConflictError struct{}

func (ConflictError) Error() string   { return "conflict" } // want Error:"."
func (ConflictError) StatusCode() int { return 409 }        // want StatusCode:"."

// DynamicError has a dynamic status, as fuego.HTTPError does: 500 when Status is 0.
type DynamicError struct{ Status int }

func (e DynamicError) Error() string { return "dynamic" } // want Error:"."
func (e DynamicError) StatusCode() int { // want StatusCode:"."
	if e.Status == 0 {
		return 500
	}
	return e.Status
}

// ToStatus maps the conflict sentinel to 409 and returns everything else as is.
//
//effectgo:errmap
func ToStatus(err error) error { // want ToStatus:"." ToStatus:"."
	if errors.Is(err, domain.ErrConflict) {
		return ConflictError{}
	}
	return err
}

// onlyOpaque returns an unclassified error and nothing else: 500.
//
//effectgo:handler ToStatus
func onlyOpaque() func(fake.Ctx[string, string]) (int, error) { // want onlyOpaque:"."
	return func(c fake.Ctx[string, string]) (int, error) {
		return 0, fmt.Errorf(unclassifiedFormat, 1)
	}
}

// onlyDynamic returns a dynamic error: its status is unknown (default).
//
//effectgo:handler ToStatus
func onlyDynamic() func(fake.Ctx[string, string]) (int, error) { // want onlyDynamic:"."
	return func(c fake.Ctx[string, string]) (int, error) {
		return 0, DynamicError{}
	}
}

// conflictAndOpaque is the case that the old "default" mapping hid: the errmap
// maps the conflict to 409, and the handler can also return an unclassified
// error, which is 500 (not the conflict's status).
//
//effectgo:handler ToStatus
func conflictAndOpaque(svc int) func(fake.Ctx[string, string]) (int, error) { // want conflictAndOpaque:"."
	return func(c fake.Ctx[string, string]) (int, error) {
		if svc == 0 {
			return 0, domain.ErrConflict
		}
		return 0, fmt.Errorf(unclassifiedFormat, svc)
	}
}

// mixed returns all three kinds of error: the conflict (409), an unclassified
// error (500) and a dynamic status (default).
//
//effectgo:handler ToStatus
func mixed(svc int) func(fake.Ctx[string, string]) (int, error) { // want mixed:"."
	return func(c fake.Ctx[string, string]) (int, error) {
		switch svc {
		case 0:
			return 0, domain.ErrConflict
		case 1:
			return 0, DynamicError{}
		}
		return 0, fmt.Errorf(unclassifiedFormat, svc)
	}
}

// freshOnly returns an error created in place: a fresh variant, which the
// errmap cannot match, so it is 500 as well (see freshOnly in opaque_test.go).
//
//effectgo:handler ToStatus
func freshOnly() func(fake.Ctx[string, string]) (int, error) { // want freshOnly:"."
	return func(c fake.Ctx[string, string]) (int, error) {
		return 0, errors.New("created here")
	}
}

// Register registers one route per case.
func Register(s *fake.Server) { // want Register:"."
	fake.Put(s, "/opaque", onlyOpaque())
	fake.Put(s, "/dynamic", onlyDynamic())
	fake.Put(s, "/conflict-and-opaque", conflictAndOpaque(0))
	fake.Put(s, "/mixed", mixed(0))
	fake.Put(s, "/fresh", freshOnly())
}
