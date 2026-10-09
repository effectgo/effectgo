// Package poly calls the higher-order helpers of polylib with literals, named
// functions and its own parameters.
package poly

import (
	"time"

	"polylib"
)

func clockFn(t *polylib.T) error { // want clockFn:`map\[time:`
	_ = time.Now()
	return nil
}

func pureFn(t *polylib.T) error { // want pureFn:`map\[\]`
	return nil
}

// CallsNamedPure passes a named pure function: stays pure.
//
//effectgo:pure
func CallsNamedPure(id int) error { // want CallsNamedPure:`map\[\]`
	return polylib.Modify(id, pureFn)
}

// CallsNamedClock passes a named function that reads the clock.
//
//effectgo:pure
func CallsNamedClock(id int) error { // want CallsNamedClock:`map\[time:` `CallsNamedClock is declared //effectgo:pure but has effect time via CallsNamedClock → polylib\.Modify → clockFn → time\.Now`
	return polylib.Modify(id, clockFn)
}

// CallsLiteralClock passes a literal that reads the clock: the literal's body
// belongs to this function.
//
//effectgo:pure
func CallsLiteralClock(id int) error { // want CallsLiteralClock:`map\[time:` `CallsLiteralClock is declared //effectgo:pure but has effect time via CallsLiteralClock → polylib\.Modify → func literal \(poly\.go:39\) → time\.Now`
	return polylib.Modify(id, func(t *polylib.T) error {
		_ = time.Now()
		return nil
	})
}

// CallsLiteralPure passes a literal that does nothing: pure.
//
//effectgo:pure
func CallsLiteralPure(id int) error { // want CallsLiteralPure:`map\[\]`
	return polylib.Modify(id, func(t *polylib.T) error { return nil })
}

// CallsUnknown passes the result of a call, which is unknown. It also calls
// its own parameter, which is the placeholder param:1.
//
//effectgo:pure
func CallsUnknown(id int, mk func() func(*polylib.T) error) error { // want CallsUnknown:`map\[param:1:` `CallsUnknown is declared //effectgo:pure but has effect unknown via CallsUnknown → polylib\.Modify → unknown argument mk\(\)`
	return polylib.Modify(id, mk())
}

// ViaOuter passes a named clock function through polylib.Outer: the
// placeholder composes through two calls.
//
//effectgo:pure
func ViaOuter() error { // want ViaOuter:`map\[time:` `ViaOuter is declared //effectgo:pure but has effect time via ViaOuter → polylib\.Outer → polylib\.Modify → clockFn → time\.Now`
	return polylib.Outer(clockFn)
}

// ViaOuterOK is the same call under a contract that allows time.
//
//effectgo:allow time
func ViaOuterOK() error { // want ViaOuterOK:`map\[time:`
	return polylib.Outer(clockFn)
}

// Pass forwards its own parameter through polylib.Outer: param:0 composes.
//
//effectgo:pure
func Pass(fn func(*polylib.T) error) error { // want Pass:`map\[param:0:`
	return polylib.Outer(fn)
}

// SumAll uses Each with a literal that writes a local: pure.
//
//effectgo:pure
func SumAll(xs []int) int { // want SumAll:`map\[\]`
	total := 0
	polylib.Each(xs, func(x int) { total += x })
	return total
}

// Box is a receiver for the method-expression case.
type Box struct{ n int }

// Apply calls f with its receiver. Effects: param:0 (the receiver is not a parameter).
func (b Box) Apply(f func(Box) int) int { // want Apply:`map\[param:0:`
	return f(b)
}

func boxClock(b Box) int { // want boxClock:`map\[time:`
	_ = time.Now()
	return b.n
}

// CallsMethodExpr passes a named clock function through a method expression.
// The receiver argument is not the func argument: the offset is one.
//
//effectgo:pure
func CallsMethodExpr(b Box) int { // want CallsMethodExpr:`map\[time:` `CallsMethodExpr is declared //effectgo:pure but has effect time via CallsMethodExpr → Box\.Apply → boxClock → time\.Now`
	return Box.Apply(b, boxClock)
}

// CallsRepeatClock passes a clock literal to Repeat, which recurses through
// its parameter.
//
//effectgo:pure
func CallsRepeatClock() { // want CallsRepeatClock:`map\[time:` `CallsRepeatClock is declared //effectgo:pure but has effect time via CallsRepeatClock → polylib\.Repeat → func literal \(poly\.go:117\) → time\.Now`
	polylib.Repeat(2, func() { _ = time.Now() })
}
