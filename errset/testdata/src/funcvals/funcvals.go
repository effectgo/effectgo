// Package funcvals checks error stubs for a package-level func variable and a
// named func type (testdata/stubs/funcvals.stubs), and the error set of a
// func literal that is called in place.
package funcvals

import "errors"

// ErrBoom is the error that the stubbed call variants return.
var ErrBoom = errors.New("boom")

// ErrOther is the contract that the functions below violate.
var ErrOther = errors.New("other")

// Gate is a package-level func variable. The stub says its calls return ErrBoom.
var Gate = func() error { return nil }

// Action is a named func type. The stub says every call through it returns ErrBoom.
type Action func() error

//effectgo:errors ErrOther
func viaGate() error { return Gate() } // want "viaGate may return funcvals.ErrBoom" viaGate:"."

//effectgo:errors ErrOther
func viaAction(a Action) error { return a() } // want "viaAction may return funcvals.ErrBoom" viaAction:"."

// A literal called in place: its own returns are the call's error set.
//
//effectgo:errors ErrOther
func immediate() error { // want "immediate may return funcvals.ErrBoom" immediate:"."
	return func() error { return ErrBoom }()
}
