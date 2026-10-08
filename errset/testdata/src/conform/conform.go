// Package conform declares types that implement contracted interfaces.
package conform

import "ifacedef"

type statusErr struct{ code int }

func (statusErr) Error() string     { return "status" } // want Error:"."
func (e statusErr) StatusCode() int { return e.code }   // want StatusCode:"."

// BadGetter violates Getter.Get: ErrConflict is neither listed nor a status error.
type BadGetter struct{}

func (*BadGetter) Get(id int) error { // want Get:"." "\\(\\*BadGetter\\).Get violates contract of \\(Getter\\).Get: may return ifacedef.ErrConflict"
	return ifacedef.ErrConflict
}

// OKGetter satisfies Getter.Get: a listed sentinel and a status error.
type OKGetter struct{}

func (OKGetter) Get(id int) error { // want Get:"."
	if id > 0 {
		return ifacedef.ErrNotFound
	}
	return statusErr{code: 400}
}

// Mixer implements Mixed. Its Put returns ErrNotFound, which the method-level
// contract on Mixed.Put does not allow (the type-level one would).
type Mixer struct{}

func (Mixer) Get(id int) error { return ifacedef.ErrNotFound } // want Get:"."

func (Mixer) Put(id int, v string) error { // want Put:"." "\\(Mixer\\).Put violates contract of \\(Mixed\\).Put: may return ifacedef.ErrNotFound"
	return ifacedef.ErrNotFound
}
