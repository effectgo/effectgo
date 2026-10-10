// Package domain holds sentinel errors, as the todo example does.
package domain

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// ValidationErr has no status method: it is a plain error.
type ValidationErr struct{ Field string }

func (e ValidationErr) Error() string { return "invalid " + e.Field }
