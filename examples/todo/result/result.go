// Package result is a small Rust-style Result[T] type.
//
// A Result holds either a value of type T or a non-nil error. Because Go
// forbids methods with their own type parameters, Map, AndThen and MapErr are
// package-level functions.
package result

import "fmt"

// Result is either Ok(val) or Err(err).
type Result[T any] struct {
	val T
	err error
}

// Ok wraps a successful value.
func Ok[T any](v T) Result[T] {
	return Result[T]{val: v}
}

// Err wraps an error. It panics if err is nil, because a nil error would
// make the Result look like Ok with a zero value.
func Err[T any](err error) Result[T] {
	if err == nil {
		panic("result.Err: nil error")
	}
	return Result[T]{err: err}
}

// From converts the (value, error) pair idiomatic Go returns into a Result.
func From[T any](v T, err error) Result[T] {
	if err != nil {
		return Err[T](err)
	}
	return Ok(v)
}

// IsOk reports whether r holds a value.
func (r Result[T]) IsOk() bool {
	return r.err == nil
}

// Get returns the value and error in idiomatic Go form.
func (r Result[T]) Get() (T, error) {
	return r.val, r.err
}

// Unwrap returns the value and panics if r is an Err.
func (r Result[T]) Unwrap() T {
	if r.err != nil {
		panic(fmt.Sprintf("result: Unwrap on Err: %v", r.err))
	}
	return r.val
}

// UnwrapOr returns the value, or def if r is an Err.
func (r Result[T]) UnwrapOr(def T) T {
	if r.err != nil {
		return def
	}
	return r.val
}

// Map applies f to the value of an Ok, leaving an Err untouched.
func Map[T, U any](r Result[T], f func(T) U) Result[U] {
	if r.err != nil {
		return Err[U](r.err)
	}
	return Ok(f(r.val))
}

// AndThen chains a fallible step: f runs only if r is Ok.
func AndThen[T, U any](r Result[T], f func(T) Result[U]) Result[U] {
	if r.err != nil {
		return Err[U](r.err)
	}
	return f(r.val)
}

// MapErr transforms the error of an Err, leaving an Ok untouched.
func MapErr[T any](r Result[T], f func(error) error) Result[T] {
	if r.err == nil {
		return r
	}
	return Err[T](f(r.err))
}
