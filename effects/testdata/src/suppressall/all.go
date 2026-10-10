// Package suppressall is the -all counterpart of package suppress: with the
// flag, the callers of a violating function are reported too.
package suppressall

import (
	"suppressdep"
	"time"
)

// Inner breaks its contract.
//
//effectgo:pure
func Inner() string { return time.Now().String() } // want Inner:`map\[time:` `Inner is declared //effectgo:pure but has effect time via Inner → time\.Now`

// Outer is reported with -all.
//
//effectgo:pure
func Outer() string { return Inner() } // want Outer:`map\[time:` `Outer is declared //effectgo:pure but has effect time via Outer → Inner → time\.Now`

// CallsBad is reported with -all.
//
//effectgo:pure
func CallsBad() string { return suppressdep.Bad() } // want CallsBad:`map\[time:` `CallsBad is declared //effectgo:pure but has effect time via CallsBad → suppressdep\.Bad → time\.Now`
