package httpres

import (
	"errors"

	"domain"
)

// StatusError is implemented by errors that carry an HTTP status.
type StatusError interface {
	error
	StatusCode() int
}

//effectgo:errmap
func ToHTTPError(err error) error {
	var se StatusError
	if errors.As(err, &se) {
		return err
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return notFound{}
	case errors.Is(err, domain.ErrInvalid):
		return invalid{}
	default:
		return err
	}
}

// notFound and invalid are the statuses that ToHTTPError gives the mapped
// variants. Returning err itself would be a passthrough, and a sentinel has no
// status of its own (see errset/passthrough).
type notFound struct{}

func (notFound) Error() string   { return "not found" }
func (notFound) StatusCode() int { return 404 }

type invalid struct{}

func (invalid) Error() string   { return "invalid" }
func (invalid) StatusCode() int { return 400 }

//effectgo:errmap
func MapTypeOnly(err error) error {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		return err
	}
	return err
}
