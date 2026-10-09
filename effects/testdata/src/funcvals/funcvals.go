// Package funcvals checks stub rules for named func types and for package-level
// func variables (testdata/stubs/funcvals.stubs gives the rules).
package funcvals

import (
	"context"
	"time"
)

// Hook is a package-level func variable. Its default does I/O (time), but the
// stub file says calls through it are pure.
var Hook = func() { _ = time.Now() }

// Stop calls a context.CancelFunc. The stub says every call through the type is
// pure.
//
//effectgo:pure
func Stop(cancel context.CancelFunc) { // want Stop:`map\[\]`
	cancel()
}

// CallHook calls the package variable. The stub says it is pure.
//
//effectgo:pure
func CallHook() { // want CallHook:`map\[\]`
	Hook()
}

// Unstubbed calls a func value that no stub names: unknown.
//
//effectgo:pure
func Unstubbed(f func()) { // want Unstubbed:`map\[param:0:`
	f()
}
