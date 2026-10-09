package ssaflow

import (
	"errors"
	"fmt"
)

// VE is an error type without an Unwrap method; Wrap wraps another error.
type VE struct{ Reason string }

func (v *VE) Error() string { return v.Reason } // want Error:"."

type Wrap struct{ Err error }

func (w *Wrap) Error() string { return "wrap" } // want Error:"."
func (w *Wrap) Unwrap() error { return w.Err }  // want Unwrap:"." "errors of Wrap.Unwrap: \\?"

// Coder is an interface that CodedErr implements and VE does not.
type Coder interface{ Code() int }

type CodedErr struct{}

func (CodedErr) Error() string { return "coded" } // want Error:"."
func (CodedErr) Code() int     { return 7 }       // want Code:"."

func getNF() error    { return ErrA }             // want getNF:"." "errors of getNF: ssaflow.ErrA"
func getVE() error    { return &VE{"x"} }         // want getVE:"." "errors of getVE: ssaflow.VE"
func getW() error     { return &Wrap{Err: ErrA} } // want getW:"." "errors of getW: ssaflow.Wrap"
func getCoded() error { return CodedErr{} }       // want getCoded:"." "errors of getCoded: ssaflow.CodedErr"

// errors.Is true keeps the sentinel and the wrappers; false removes the sentinel.
func isKeep(n int) error { // want isKeep:"." "errors of isKeep: ssaflow.ErrA | ssaflow.Wrap"
	var err error
	if n == 0 {
		err = getNF()
	} else if n == 1 {
		err = getVE()
	} else {
		err = getW()
	}
	if errors.Is(err, ErrA) {
		return err
	}
	return nil
}

func isGuard(n int) error { // want isGuard:"." "errors of isGuard: ssaflow.VE"
	var err error
	if n == 0 {
		err = getNF()
	} else {
		err = getVE()
	}
	if errors.Is(err, ErrA) {
		return nil
	}
	return err
}

// The nil edge of err == nil carries nothing.
func nilEdge(n int) error { // want nilEdge:"." "errors of nilEdge: ssaflow.ErrA"
	var err error
	if n > 0 {
		err = getNF()
	}
	if err == nil {
		return err
	}
	return err
}

// errors.As with a concrete type: true keeps it, false removes it.
func asGuard(n int) error { // want asGuard:"." "errors of asGuard: ssaflow.Wrap"
	var err error
	if n == 0 {
		err = getVE()
	} else {
		err = getW()
	}
	var ve *VE
	if errors.As(err, &ve) {
		return nil
	}
	return err
}

func asTrue(n int) error { // want asTrue:"." "errors of asTrue: ssaflow.VE | ssaflow.Wrap"
	var err error
	if n == 0 {
		err = getVE()
	} else if n == 1 {
		err = getW()
	} else {
		err = getNF()
	}
	var ve *VE
	if errors.As(err, &ve) {
		return err
	}
	return nil
}

// errors.As with an interface: false removes the variants that implement it.
func ifaceAs(n int) error { // want ifaceAs:"." "errors of ifaceAs: ssaflow.VE"
	var err error
	if n == 0 {
		err = getCoded()
	} else {
		err = getVE()
	}
	var c Coder
	if errors.As(err, &c) {
		return nil
	}
	return err
}

// A predicate with a fact: IsVE is errors.As with *VE on its parameter.
func IsVE(err error) bool { // want IsVE:"errset-test: errors.As ssaflow.VE" IsVE:"errors=\\(none\\)"
	var ve *VE
	return errors.As(err, &ve)
}

func predGuard(n int) error { // want predGuard:"." "errors of predGuard: ssaflow.ErrA"
	var err error
	if n == 0 {
		err = getVE()
	} else {
		err = getNF()
	}
	if IsVE(err) {
		return nil
	}
	return err
}

// A type assertion with the comma-ok form narrows like errors.As, no chain.
func assertGuard(n int) error { // want assertGuard:"." "errors of assertGuard: ssaflow.ErrA"
	var err error
	if n == 0 {
		err = getVE()
	} else {
		err = getNF()
	}
	if _, ok := err.(*VE); ok {
		return nil
	}
	return err
}

// A switch is an If chain: the default case has the sentinel removed.
func switchGuard(n int) error { // want switchGuard:"." "errors of switchGuard: ssaflow.VE"
	var err error
	if n == 0 {
		err = getNF()
	} else {
		err = getVE()
	}
	switch {
	case errors.Is(err, ErrA):
		return nil
	default:
		return fmt.Errorf("wrap: %w", err)
	}
}

// A negated test swaps the edges.
func notGuard(n int) error { // want notGuard:"." "errors of notGuard: ssaflow.VE"
	var err error
	if n == 0 {
		err = getNF()
	} else {
		err = getVE()
	}
	if !errors.Is(err, ErrA) {
		return err
	}
	return nil
}

// The reset in the guard reaches the return as nothing.
func guardReset() error { // want guardReset:"." "errors of guardReset: \\(none\\)"
	err := getNF()
	if errors.Is(err, ErrA) {
		err = nil
	}
	return err
}

func lookup(n int) error { // want lookup:"." "errors of lookup: ssaflow.ErrA | ssaflow.VE"
	if n == 0 {
		return getNF()
	}
	return getVE()
}

// A callee's set is narrowed by the guard of its caller.
func handled(n int) error { // want handled:"." "errors of handled: ssaflow.VE"
	err := lookup(n)
	if errors.Is(err, ErrA) {
		return nil
	}
	return err
}
