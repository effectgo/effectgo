// Package limits documents current behavior in known-hard cases.
package limits

import (
	"errors"
	"os"
	"time"
)

// Box is a generic type; its method is analyzed best-effort.
type Box[T any] struct{ v T }

func (b *Box[T]) Set(v T) { // want Set:`map\[time:\[\{limits limits \(\*Box\)\.Set\} \{time time Now\}\]\]`
	b.v = v
	_ = time.Now()
}

func Use[T any](v T) { // want Use:`map\[\]`
	_ = v
}

// Clock is a generic function that reads the clock.
func Clock[T any]() T { // want Clock:`map\[time:`
	var z T
	_ = time.Now()
	return z
}

//effectgo:pure
func Gen() { // want Gen:`map\[time:` `Gen is declared //effectgo:pure but has effect time via Gen → \(\*Box\)\.Set → time\.Now`
	var b Box[int]
	b.Set(1)
	_ = Clock[int]()
}

// MethodValue: a method or function value that is not called is not an effect.
//
//effectgo:pure
func MethodValue() { // want MethodValue:`map\[\]`
	f := time.Now
	_ = f
}

// ErrStr: error.Error is an interface method, so it is reported as unknown
// even though the stdlib error implementations are not modeled.
//
//effectgo:pure
func ErrStr(err error) string { // want ErrStr:`map\[unknown:` `ErrStr is declared //effectgo:pure but has effect unknown via ErrStr → error\.Error`
	return err.Error()
}

// Builtin: the print and println builtins are not in the taxonomy.
//
//effectgo:pure
func Builtin() { // want Builtin:`map\[\]`
	println("x")
}

// Readers: os.Args is env; errors.New is stdlib with no taxonomy entry.
//
//effectgo:pure
func Readers() { // want Readers:`map\[env:` `Readers is declared //effectgo:pure but has effect env via Readers → os\.Args`
	_ = errors.New("x")
	_ = os.Args
}
