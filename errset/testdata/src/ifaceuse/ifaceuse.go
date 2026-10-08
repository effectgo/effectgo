// Package ifaceuse calls contracted interface methods.
package ifaceuse

import "ifacedef"

// A call yields the declared set, so the contract holds.
//
//effectgo:errors ifacedef.ErrNotFound, iface:ifacedef.Status
func getOK(g ifacedef.Getter) error { return g.Get(1) } // want getOK:"."

// The call yields ErrNotFound and some ifacedef.Status, neither allowed by ErrConflict.
//
//effectgo:errors ifacedef.ErrConflict
func getBad(g ifacedef.Getter) error { return g.Get(1) } // want getBad:"." "getBad may return ifacedef.ErrNotFound \\(via \\(Getter\\).Get \\[contract\\]\\), not in its" "getBad may return some ifacedef.Status \\(via \\(Getter\\).Get \\[contract\\]\\), not in its"

//effectgo:errors ifacedef.ErrNotFound
func putBad(p ifacedef.Putter) error { return p.Put(1, "x") } // want putBad:"." "putBad may return ifacedef.ErrConflict \\(via \\(Putter\\).Put \\[contract\\]\\), not in its"

//effectgo:errors ifacedef.ErrNotFound, ifacedef.ErrConflict
func putOK(p ifacedef.Putter) error { return p.Put(1, "x") } // want putOK:"."

// The method-level directive on Mixed.Put overrides the type-level one.
//
//effectgo:errors ifacedef.ErrNotFound
func mixedPut(m ifacedef.Mixed) error { return m.Put(1, "x") } // want mixedPut:"." "mixedPut may return ifacedef.ErrConflict \\(via \\(Mixed\\).Put \\[contract\\]\\), not in its"

// Mixed.Get inherits the type-level contract.
//
//effectgo:errors ifacedef.ErrNotFound
func mixedGet(m ifacedef.Mixed) error { return m.Get(1) } // want mixedGet:"."

//effectgo:errors ?
func loadOK(l ifacedef.Loader) error { return l.Load("x") } // want loadOK:"."

// No contract: the call is opaque and nothing is checked.
func plain(p ifacedef.Plain) error { return p.Do() } // want plain:"."
