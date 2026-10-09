// Package service is a project package with domain errors.
package service

import "errors"

// ErrNotFound is a domain sentinel.
var ErrNotFound = errors.New("not found")

// ErrConflict is a domain sentinel that no mapper maps.
var ErrConflict = errors.New("conflict")

// ValidationError is a typed domain error.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Store is a fake service.
type Store struct{}

// Find returns ErrNotFound for a missing row.
func (Store) Find(id int) (int, error) {
	if id < 0 {
		return 0, ErrNotFound
	}
	return id, nil
}

// Save returns ErrConflict or a validation error.
func (Store) Save(id int) error {
	if id == 0 {
		return ErrConflict
	}
	if id < 0 {
		return &ValidationError{Msg: "bad id"}
	}
	return nil
}

// BadInput is a typed input error, tested by IsBad.
type BadInput struct{}

func (*BadInput) Error() string { return "bad input" }

// IsBad reports whether err is a BadInput (errors.As on the typed error).
func IsBad(err error) bool {
	var b *BadInput
	return errors.As(err, &b)
}

// Check returns a BadInput for id 0.
func (Store) Check(id int) error {
	if id == 0 {
		return &BadInput{}
	}
	return nil
}
