// Package httpres adapts Result-returning handlers to Fuego's (T, error)
// handler shape, and holds the single place where domain errors become HTTP
// errors.
package httpres

import (
	"errors"

	"github.com/effectgo/effectgo/examples/todo/domain"
	"github.com/effectgo/effectgo/examples/todo/result"
	"github.com/go-fuego/fuego"
)

// ToHTTPError is the one central mapping from domain errors to HTTP errors.
//
//   - errors that already carry an HTTP status (fuego's own, e.g. a body
//     decode failure) are passed through unchanged;
//   - domain.ErrNotFound -> 404; domain.ErrInvalid -> 400;
//   - anything else is returned unchanged and Fuego reports it as 500.
//
//effectgo:errmap
func ToHTTPError(err error) error {
	var withStatus fuego.ErrorWithStatus
	if errors.As(err, &withStatus) {
		return err
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return fuego.NotFoundError{Err: err, Detail: err.Error()}
	case errors.Is(err, domain.ErrInvalid):
		return fuego.BadRequestError{Err: err, Detail: err.Error()}
	default:
		return err
	}
}

// Wrap turns a handler returning Result[T] into a handler returning (T, error)
// that Fuego can register. It works for any body type B and params type P.
func Wrap[T, B, P any](h func(fuego.Context[B, P]) result.Result[T]) func(fuego.Context[B, P]) (T, error) {
	return func(c fuego.Context[B, P]) (T, error) {
		v, err := h(c).Get()
		if err != nil {
			var zero T
			return zero, ToHTTPError(err)
		}
		return v, nil
	}
}

// NoBody is Wrap specialised to routes with no typed body and no typed params
// (fuego.ContextNoBody).
func NoBody[T any](h func(fuego.ContextNoBody) result.Result[T]) func(fuego.ContextNoBody) (T, error) {
	return Wrap(h)
}

// WithBody is Wrap specialised to routes with a typed body B and no typed
// params (fuego.ContextWithBody[B]).
func WithBody[T, B any](h func(fuego.ContextWithBody[B]) result.Result[T]) func(fuego.ContextWithBody[B]) (T, error) {
	return Wrap(h)
}
