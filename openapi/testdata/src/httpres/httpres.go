// Package httpres holds the errmap of the test: the one place that maps domain
// errors to HTTP errors.
package httpres

import (
	"errors"

	"domain"
)

// NotFound has a constant status: 404.
type NotFound struct{ Err error }

func (NotFound) Error() string   { return "not found" }
func (NotFound) StatusCode() int { return 404 }

// HTTPError has a dynamic status, as fuego.HTTPError does: Status 0 means 500.
type HTTPError struct {
	Status int
	Err    error
}

func (e HTTPError) Error() string { return "http error" }
func (e HTTPError) StatusCode() int {
	if e.Status == 0 {
		return 500
	}
	return e.Status
}

// StatusError is the interface that ToHTTPError passes through.
type StatusError interface {
	error
	StatusCode() int
}

// ToHTTPError maps domain errors to HTTP errors.
//
//effectgo:errmap
func ToHTTPError(err error) error {
	var se StatusError
	if errors.As(err, &se) {
		return err
	}
	if errors.Is(err, domain.ErrNotFound) {
		return NotFound{Err: err}
	}
	if errors.Is(err, domain.ErrConflict) {
		return HTTPError{Status: 409, Err: err}
	}
	return err
}
