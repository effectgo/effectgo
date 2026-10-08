// Package ifaceweb checks handlers whose errors are interface variants.
package ifaceweb

import (
	"ifacedef"
	"ifacemap"
)

// Keeps the ifacemap import used by the //effectgo:handler directives below.
var _ = ifacemap.Map

// Covered by Map: ErrNotFound by key, some ifacedef.Status by errors.As.
//
//effectgo:handler ifacemap.Map
func getHandler(g ifacedef.Getter) func() error { // want getHandler:"."
	return func() error { return g.Get(1) }
}

// Strict does not cover some ifacedef.Status.
//
//effectgo:handler ifacemap.Strict
func strictHandler(g ifacedef.Getter) func() error { // want strictHandler:"." "handler strictHandler may return some ifacedef.Status \\(via \\(Getter\\).Get \\[contract\\]\\), which errmap ifacemap.Strict does not map"
	return func() error { return g.Get(1) }
}

// Loose covers the status interface by method-set inclusion.
//
//effectgo:handler ifacemap.Loose
func looseHandler(g ifacedef.Getter) func() error { // want looseHandler:"."
	return func() error { return g.Get(1) }
}

// ErrConflict is not mapped by Map.
//
//effectgo:handler ifacemap.Map
func putHandler(p ifacedef.Putter) func() error { // want putHandler:"." "handler putHandler may return ifacedef.ErrConflict \\(via \\(Putter\\).Put \\[contract\\]\\), which errmap ifacemap.Map does not map"
	return func() error { return p.Put(1, "x") }
}
