// Package embedlimit pins a limit of the error-set conformance check. A method that a
// type promotes from an embedded interface is not checked against the error contract
// of the contracted interface that the type satisfies, although the same method
// declared on the type is checked (direct). The effects analyzer checks the promoted
// method (effects/testdata/src/promoted); the error-set check does not, because an
// interface method has no inferred error set to compare.
package embedlimit

import "errors"

// ErrA is the only error a Putter may return.
var ErrA = errors.New("a")

// Putter is contracted: Put may return only ErrA.
//
//effectgo:errors ErrA
type Putter interface {
	Put() error // want Put:"."
}

// Inner is uncontracted: its Put may return anything.
type Inner interface {
	Put() error
}

// outer promotes Put from Inner. It is not reported: this is the limit.
type outer struct {
	Inner
}

// NewOuter returns outer as a Putter.
func NewOuter(i Inner) Putter { return outer{Inner: i} } // want NewOuter:"."

// direct declares Put itself, so its Put is checked and reported.
type direct struct{}

func (direct) Put() error { return errors.New("x") } // want Put:"." "\\(direct\\)\\.Put violates contract of \\(Putter\\)\\.Put: may return new@embedlimit\\.direct\\.Put"

// NewDirect returns direct as a Putter.
func NewDirect() Putter { return direct{} } // want NewDirect:"."
