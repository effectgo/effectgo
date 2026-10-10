// Package huma is a fake of github.com/danielgtaylor/huma/v2 for the adapter
// tests. It mirrors the shapes the stub rules name: the StatusError interface,
// the status constructors (which call the NewError variable, as the real ones do),
// the route functions and huma.Register.
package huma

import (
	"context"
	"net/http"
)

// API is the registration target.
type API interface {
	Name() string
}

// Operation describes a route for Register.
type Operation struct {
	Method string
	Path   string
}

// StatusError is an error with an HTTP status.
type StatusError interface {
	GetStatus() int
	Error() string
}

// ErrorModel is the default StatusError.
type ErrorModel struct {
	Status int
	Detail string
}

func (e *ErrorModel) Error() string  { return e.Detail }
func (e *ErrorModel) GetStatus() int { return e.Status }

// NewError is a variable of func type, as in the real module.
var NewError = func(status int, msg string, errs ...error) StatusError {
	return &ErrorModel{Status: status, Detail: msg}
}

// NewErrorWithContext takes a context first.
var NewErrorWithContext = func(_ context.Context, status int, msg string, errs ...error) StatusError {
	return NewError(status, msg, errs...)
}

// Error404NotFound returns a 404.
func Error404NotFound(msg string, errs ...error) StatusError {
	return NewError(http.StatusNotFound, msg, errs...)
}

// Error400BadRequest returns a 400.
func Error400BadRequest(msg string, errs ...error) StatusError {
	return NewError(http.StatusBadRequest, msg, errs...)
}

// Error409Conflict returns a 409.
func Error409Conflict(msg string, errs ...error) StatusError {
	return NewError(http.StatusConflict, msg, errs...)
}

// Error500InternalServerError returns a 500.
func Error500InternalServerError(msg string, errs ...error) StatusError {
	return NewError(http.StatusInternalServerError, msg, errs...)
}

// Get registers a GET operation.
func Get[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), ops ...func()) {
}

// Post registers a POST operation.
func Post[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), ops ...func()) {
}

// Put registers a PUT operation.
func Put[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), ops ...func()) {
}

// Delete registers a DELETE operation.
func Delete[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), ops ...func()) {
}

// Register registers the operation described by op.
func Register[I, O any](api API, op Operation, handler func(context.Context, *I) (*O, error)) {
}
