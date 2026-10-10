// Package suppressdep is a dependency of the suppress tests. Its violations
// are the roots that the callers in package suppress are reported against.
package suppressdep

import "time"

// Bad breaks its pure contract; it is the root of the violation of CallsBad.
//
//effectgo:pure
func Bad() string { return time.Now().String() } // want Bad:`map\[time:` `Bad is declared //effectgo:pure but has effect time via Bad → time\.Now`

// Plain has no contract, so a violation of its caller is the caller's own.
func Plain() string { return time.Now().String() } // want Plain:`map\[time:`

// Clock allows time, so a caller of Clock is the root of its own violation.
//
//effectgo:allow time
func Clock() string { return time.Now().String() } // want Clock:`map\[time:`
