// Package polylib has higher-order helpers whose effects depend on the func
// arguments that callers pass.
package polylib

import "sync"

// T is a value guarded by a lock.
type T struct {
	mu sync.Mutex
	N  int
}

// Modify fetches the value for id, locks it, calls fn and unlocks. Its own
// effects are only those of fn, which is the placeholder param:1.
//
//effectgo:pure
func Modify(id int, fn func(*T) error) error { // want Modify:`map\[param:1:`
	t := &T{N: id}
	t.mu.Lock()
	err := fn(t)
	t.mu.Unlock()
	return err
}

// Each calls fn on every element. Effects: param:1.
//
//effectgo:pure
func Each(xs []int, fn func(int)) { // want Each:`map\[param:1:`
	for _, x := range xs {
		fn(x)
	}
}

// Outer forwards its own parameter to Modify. The placeholder composes: the
// effect of Outer is param:0.
//
//effectgo:pure
func Outer(fn func(*T) error) error { // want Outer:`map\[param:0:`
	return Modify(1, fn)
}

// Repeat calls fn n times, recursing through its own parameter: the
// placeholder stays param:1 through the recursion.
func Repeat(n int, fn func()) { // want Repeat:`map\[param:1:`
	if n > 0 {
		fn()
		Repeat(n-1, fn)
	}
}
