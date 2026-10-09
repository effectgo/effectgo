// Package rootdep is a dependency of the root tests: its violation is the root
// of the violations of its callers in package root.
package rootdep

import "errors"

var ErrConflict = errors.New("conflict")

var ErrOther = errors.New("other")

// Bad contracts ErrOther and returns ErrConflict: its own violation.
//
//effectgo:errors ErrOther
func Bad() error { return ErrConflict }

// Plain has no contract.
func Plain() error { return ErrConflict }

// Clock allows ErrConflict.
//
//effectgo:errors ErrConflict
func Clock() error { return ErrConflict }
