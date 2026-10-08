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
		return err
	case errors.Is(err, domain.ErrInvalid):
		return err
	default:
		return err
	}
}

//effectgo:errmap
func MapTypeOnly(err error) error {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		return err
	}
	return err
}
