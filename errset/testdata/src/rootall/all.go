// Package rootall is the -all counterpart of package root: with the flag, the
// callers of a violating function are reported too.
package rootall

import "errors"

var ErrConflict = errors.New("conflict")

var ErrOther = errors.New("other")

// Inner breaks its contract.
//
//effectgo:errors ErrOther
func Inner() error { return ErrConflict } // want Inner:"." "Inner may return rootall.ErrConflict"

// Outer is reported with -all.
//
//effectgo:errors ErrOther
func Outer() error { return Inner() } // want Outer:"." "Outer may return rootall.ErrConflict"
