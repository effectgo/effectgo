// Package interact checks error sets where an immediately-invoked literal (driver
// stubs: the literal's own returns) meets fresh variants (report tooling: one
// new@ variant per creating declared function).
package interact

import (
	"errors"
	"fmt"
)

// ErrBase is a sentinel.
var ErrBase = errors.New("base")

// Immediate returns an immediately-invoked literal's fresh error. The literal is
// part of Immediate, so the variant is keyed by Immediate.
func Immediate() error { // want "errors of Immediate: new@interact.Immediate" "witness of Immediate: new@interact.Immediate via \\(direct\\)" Immediate:"errors=new@interact.Immediate"
	return func() error { return errors.New("x") }()
}

// ImmediateSentinel returns the sentinel from an immediately-invoked literal.
func ImmediateSentinel() error { // want "errors of ImmediateSentinel: interact.ErrBase" "witness of ImmediateSentinel: interact.ErrBase via \\(direct\\)" ImmediateSentinel:"errors=interact.ErrBase"
	return func() error { return ErrBase }()
}

// Wrapped wraps the sentinel from an immediately-invoked literal: the set is the
// wrapped error, not a fresh one.
func Wrapped() error { // want "errors of Wrapped: interact.ErrBase" "witness of Wrapped: interact.ErrBase via \\(direct\\)" Wrapped:"errors=interact.ErrBase"
	return fmt.Errorf("w: %w", func() error { return ErrBase }())
}

// Helper creates a fresh error in a named helper; the caller's witness names it.
func Helper() error { return helper() } // want "errors of Helper: new@interact.helper" "witness of Helper: new@interact.helper via helper" Helper:"errors=new@interact.helper"

func helper() error { return errors.New("h") } // want "errors of helper: new@interact.helper" "witness of helper: new@interact.helper via \\(direct\\)" helper:"errors=new@interact.helper"

// Stored keeps a literal that creates an error; it is not run here, so the
// function's own set is only the literal it returns.
func Stored() func() error { return func() error { return errors.New("s") } } // want "handler errors of Stored: new@interact.Stored" "witness of Stored \\[handler\\]: new@interact.Stored via \\(direct\\)" Stored:"handler=new@interact.Stored"
