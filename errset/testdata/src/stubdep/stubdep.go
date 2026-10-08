// Package stubdep is the dependency whose functions the stub file describes.
package stubdep

import "errors"

var ErrA = errors.New("a")

// Status is an error with a status code.
type Status interface {
	error
	StatusCode() int
}

// Lookup returns nothing; the stub claims ErrA and some Status.
func Lookup(id int) error { return nil }

// Hard returns an opaque error; the stub says it returns nothing.
func Hard() error { return errors.New("hard") }

// Repo's Find has a contract that the stub overrides.
type Repo interface {
	//effectgo:errors ErrA
	Find(id int) error
}
