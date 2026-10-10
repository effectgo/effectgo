// Package suppress exercises the root rule of the effects analyzer: a
// violation of a function is not reported when its only source of the effect
// is a callee that violates its own contract (see docs/reference/analyzers.md).
package suppress

import (
	"suppressdep"
	"time"
)

// Inner breaks its contract: it is the root.
//
//effectgo:pure
func Inner() string { return time.Now().String() } // want Inner:`map\[time:` `Inner is declared //effectgo:pure but has effect time via Inner → time\.Now`

// Outer's violation is the consequence of Inner's: not reported.
//
//effectgo:pure
func Outer() string { return Inner() } // want Outer:`map\[time:`

// Helper has no contract, so UsesHelper is the root of its own violation.
func Helper() string { return time.Now().String() } // want Helper:`map\[time:`

//effectgo:pure
func UsesHelper() string { return Helper() } // want UsesHelper:`map\[time:` `UsesHelper is declared //effectgo:pure but has effect time via UsesHelper → Helper → time\.Now`

// Allowed allows time, so UsesAllowed is the root.
//
//effectgo:allow time
func Allowed() string { return time.Now().String() } // want Allowed:`map\[time:`

//effectgo:pure
func UsesAllowed() string { return Allowed() } // want UsesAllowed:`map\[time:` `UsesAllowed is declared //effectgo:pure but has effect time via UsesAllowed → Allowed → time\.Now`

// Store is an uncontracted interface: a call through it is unknown.
type Store interface {
	Get() string
}

//effectgo:pure
func UsesStore(s Store) string { return s.Get() } // want UsesStore:`map\[unknown:` `UsesStore is declared //effectgo:pure but has effect unknown via UsesStore → Store\.Get`

// Stubbed has a stub rule (testdata/stubs/suppress.stubs) that gives it net.
// A stub is the root, so its caller is reported.
func Stubbed() string { return "" } // want Stubbed:`map\[\]`

//effectgo:pure
func UsesStubbed() string { return Stubbed() } // want UsesStubbed:`map\[net:` `UsesStubbed is declared //effectgo:pure but has effect net via UsesStubbed → Stubbed`

// Mixed has time from its own body as well as from Inner: its own call is a
// root, so it is reported.
//
//effectgo:pure
func Mixed() string { // want Mixed:`map\[time:` `Mixed is declared //effectgo:pure but has effect time via Mixed → time\.Now`
	_ = time.Now()
	return Inner()
}

// CallsBad's only source is suppressdep.Bad, which violates: not reported.
//
//effectgo:pure
func CallsBad() string { return suppressdep.Bad() } // want CallsBad:`map\[time:`

// CallsPlain's callee has no contract: CallsPlain is the root.
//
//effectgo:pure
func CallsPlain() string { return suppressdep.Plain() } // want CallsPlain:`map\[time:` `CallsPlain is declared //effectgo:pure but has effect time via CallsPlain → suppressdep\.Plain → time\.Now`

// CallsClock's callee allows time: CallsClock is the root.
//
//effectgo:pure
func CallsClock() string { return suppressdep.Clock() } // want CallsClock:`map\[time:` `CallsClock is declared //effectgo:pure but has effect time via CallsClock → suppressdep\.Clock → time\.Now`

// Both calls: one callee does not violate, so CallsBoth is reported.
//
//effectgo:pure
func CallsBoth() string { // want CallsBoth:`map\[time:` `CallsBoth is declared //effectgo:pure but has effect time via CallsBoth → suppressdep\.Bad → time\.Now`
	return suppressdep.Bad() + suppressdep.Plain()
}
