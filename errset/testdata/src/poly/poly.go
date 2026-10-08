// Package poly checks error polymorphism for func parameters (report flag on).
package poly

import "errors"

var (
	ErrA = errors.New("a")
	ErrB = errors.New("b")
)

func apply(fn func() error) error { // want apply:"." "errors of apply: param:0"
	return fn()
}

func failB() error { // want failB:"." "errors of failB: poly.ErrB"
	return ErrB
}

// A literal argument contributes the errors its own returns produce.
func viaLiteral() error { // want viaLiteral:"." "errors of viaLiteral: poly.ErrA"
	return apply(func() error { return ErrA })
}

// A named function argument contributes its fact.
func viaNamed() error { // want viaNamed:"." "errors of viaNamed: poly.ErrB"
	return apply(failB)
}

// The caller's own parameter composes: param:0 stays param:0.
func wrap(fn func() error) error { // want wrap:"." "errors of wrap: param:0"
	return apply(fn)
}

// A literal that calls the caller's parameter composes too.
func wrapLit(fn func() error) error { // want wrapLit:"." "errors of wrapLit: param:0"
	return apply(func() error { return fn() })
}

// An argument that is a call result cannot be followed: "?".
func unknownArg(mk func() func() error) error { // want unknownArg:"." "errors of unknownArg: \\?"
	return apply(mk())
}

// param:1 is bound to the second argument, not the first.
func pick(a, b func() error) error { // want pick:"." "errors of pick: param:1"
	_ = a
	return b()
}

func viaSecond() error { // want viaSecond:"." "errors of viaSecond: poly.ErrA"
	return pick(failB, func() error { return ErrA })
}
