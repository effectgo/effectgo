// Package closure checks which func literals are charged to the declared
// function that contains them. A literal is charged when the function runs it
// (called in place, deferred, started with go, or called through a local
// variable it is the only value of). A literal that is passed, stored,
// returned or sent is charged only through the parameter it is passed to.
//
// Every function here declares //effectgo:pure, so a charged time effect is a
// violation, reported on the function's name.
package closure

import (
	"sort"
	"time"
)

// Immediate runs its literal in place: the literal's time effect is charged.
//
//effectgo:pure
func Immediate() { // want Immediate:`map\[time:` `Immediate is declared //effectgo:pure but has effect time`
	func() { _ = time.Now() }()
}

// Deferred runs its literal when it returns.
//
//effectgo:pure
func Deferred() { // want Deferred:`map\[time:` `Deferred is declared //effectgo:pure but has effect time`
	defer func() { _ = time.Now() }()
}

// Started runs its literal in a goroutine: spawn and time are charged.
//
//effectgo:pure
func Started() { // want Started:`map\[spawn:` `Started is declared //effectgo:pure but has effect spawn` `Started is declared //effectgo:pure but has effect time`
	go func() { _ = time.Now() }()
}

// Local binds the literal to a variable that it then calls.
//
//effectgo:pure
func Local() { // want Local:`map\[time:` `Local is declared //effectgo:pure but has effect time`
	f := func() { _ = time.Now() }
	f()
}

// LocalReassigned binds a variable twice. The variable is not known to hold
// the literal at the call, so the literal is not charged; the call itself is
// a call through a func value, which is unknown.
//
//effectgo:pure
func LocalReassigned(other func()) { // want LocalReassigned:`map\[unknown:` `LocalReassigned is declared //effectgo:pure but has effect unknown`
	f := func() { _ = time.Now() }
	f = other
	f()
}

// call runs its parameter.
func call(f func()) { f() } // want call:`map\[param:0:`

// Passed hands its literal to a callee that calls the parameter: the literal's
// time effect comes in through the parameter.
//
//effectgo:pure
func Passed() { // want Passed:`map\[time:` `Passed is declared //effectgo:pure but has effect time`
	call(func() { _ = time.Now() })
}

// Router stores the handlers it is given. It never runs them.
type Router struct{ handlers []func() }

// Add stores a handler.
func (r *Router) Add(f func()) { r.handlers = append(r.handlers, f) } // want Add:`map\[\]`

// Registered gives a literal to a function that only stores it. Nothing is
// charged to Registered.
//
//effectgo:pure
func Registered(r *Router) { // want Registered:`map\[\]`
	r.Add(func() { _ = time.Now() })
}

// Stored puts a literal in a struct field. Nothing is charged.
//
//effectgo:pure
func Stored() *Router { // want Stored:`map\[\]`
	return &Router{handlers: []func(){func() { _ = time.Now() }}}
}

// Returned returns a literal. Nothing is charged to the function that returns it.
//
//effectgo:pure
func Returned() func() { // want Returned:`map\[\]`
	return func() { _ = time.Now() }
}

// Unknown passes a literal to the standard library, whose body is not analyzed.
// The literal may be run by the callee, so its effect is charged.
//
//effectgo:pure
func Unknown(xs []int) { // want Unknown:`map\[time:` `Unknown is declared //effectgo:pure but has effect time`
	sort.Slice(xs, func(i, j int) bool {
		_ = time.Now()
		return xs[i] < xs[j]
	})
}

// Dynamic passes a literal to a func value of its own parameter. The callee is
// not known, so the literal is charged, and the call is the placeholder.
//
//effectgo:pure
func Dynamic(fn func(func())) { // want Dynamic:`map\[param:0:.*time:` `Dynamic is declared //effectgo:pure but has effect time`
	fn(func() { _ = time.Now() })
}

// Pure runs a literal that does nothing.
//
//effectgo:pure
func Pure() int { // want Pure:`map\[\]`
	x := 0
	func() { x++ }()
	return x
}

// StoredCaller stores a literal that calls a local literal. The local literal
// is run by the stored literal, which this function does not run, so nothing
// is charged to StoredCaller.
//
//effectgo:pure
func StoredCaller(r *Router) { // want StoredCaller:`map\[\]`
	f := func() { _ = time.Now() }
	r.Add(func() { f() })
}

// NestedRun calls a local literal from inside a literal that it runs in place.
// The local literal is run, so its time effect is charged to NestedRun.
//
//effectgo:pure
func NestedRun() { // want NestedRun:`map\[time:` `NestedRun is declared //effectgo:pure but has effect time`
	func() {
		f := func() { _ = time.Now() }
		f()
	}()
}
