// Package root exercises the root rule of the errset analyzer: a contract
// violation of a function is not reported when the variant reaches it only
// through a callee that violates its own contract (see docs/reference/analyzers.md).
package root

import (
	"errors"
	"rootdep"
)

var ErrConflict = errors.New("conflict")

var ErrOther = errors.New("other")

// Inner breaks its contract: it is the root.
//
//effectgo:errors ErrOther
func Inner() error { return ErrConflict } // want Inner:"." "Inner may return root.ErrConflict, not in its //effectgo:errors contract"

// Outer's violation is the consequence of Inner's: not reported.
//
//effectgo:errors ErrOther
func Outer() error { return Inner() } // want Outer:"."

// Helper has no contract, so UsesHelper is the root.
func Helper() error { return ErrConflict } // want Helper:"."

//effectgo:errors ErrOther
func UsesHelper() error { return Helper() } // want UsesHelper:"." "UsesHelper may return root.ErrConflict"

// Allowed allows ErrConflict, so UsesAllowed is the root.
//
//effectgo:errors ErrConflict
func Allowed() error { return ErrConflict } // want Allowed:"."

//effectgo:errors ErrOther
func UsesAllowed() error { return Allowed() } // want UsesAllowed:"." "UsesAllowed may return root.ErrConflict"

// Mixed returns ErrConflict itself as well as through Inner: a direct source,
// so it is reported.
//
//effectgo:errors ErrOther
func Mixed(n int) error { // want Mixed:"." "Mixed may return root.ErrConflict"
	if n > 0 {
		return ErrConflict
	}
	return Inner()
}

// CallsBad's only source is rootdep.Bad, which violates: not reported.
//
//effectgo:errors ErrOther
func CallsBad() error { return rootdep.Bad() } // want CallsBad:"."

// CallsPlain's callee has no contract: CallsPlain is the root.
//
//effectgo:errors ErrOther
func CallsPlain() error { return rootdep.Plain() } // want CallsPlain:"." "CallsPlain may return rootdep.ErrConflict"

// CallsClock's callee allows the variant: CallsClock is the root.
//
//effectgo:errors ErrOther
func CallsClock() error { return rootdep.Clock() } // want CallsClock:"." "CallsClock may return rootdep.ErrConflict"

// One callee violates and the other does not: reported.
//
//effectgo:errors ErrOther
func CallsBoth(n int) error { // want CallsBoth:"." "CallsBoth may return rootdep.ErrConflict"
	if n > 0 {
		return rootdep.Bad()
	}
	return rootdep.Plain()
}
