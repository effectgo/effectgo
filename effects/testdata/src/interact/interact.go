// Package interact checks witness chains where parameter polymorphism of func
// literals (driver stubs) meets witness lines (report tooling).
package interact

import "time"

// Clock is a named func type. testdata/stubs/interact.stubs gives every call
// through it the time effect.
type Clock func() time.Time

// Hook is a package-level func variable. The stub says calls through it are pure.
var Hook = func() { _ = time.Now() }

// Run calls its parameter, so a literal passed to it contributes its effects.
func Run(f func()) { f() } // want "witness of Run: param:0 via dynamic call f" Run:`map\[param:0:`

// Caller passes a literal that reads the clock. The witness names the literal.
func Caller() { Run(func() { _ = time.Now() }) } // want "witness of Caller: time via Run → func literal \\(interact.go:18\\) → time.Now" Caller:`map\[time:`

// Outer reaches the literal through Caller.
func Outer() { Caller() } // want "witness of Outer: time via Caller → Run → func literal \\(interact.go:18\\) → time.Now" Outer:`map\[time:`

// ViaClock calls a named func type value. The stub gives the call time.
func ViaClock(c Clock) time.Time { return c() } // want "witness of ViaClock: time via \\(Clock\\) call \\[stub\\]" ViaClock:`map\[time:`

// UsesClock passes time.Now through the named type.
func UsesClock() time.Time { return ViaClock(time.Now) } // want "witness of UsesClock: time via ViaClock → \\(Clock\\) call \\[stub\\]" UsesClock:`map\[time:`

// ViaHook calls the stubbed variable: pure, so no witness line.
func ViaHook() { Hook() } // want ViaHook:`map\[\]`

// Stored keeps a literal in a variable that is never called: not charged.
func Stored() func() { return func() { _ = time.Now() } } // want Stored:`map\[\]`
