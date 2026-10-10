package shape

import (
	"errors"
	"net/http"
)

var ErrX = errors.New("x")

// NoErrResult has an errors contract and no error result: the contract checks nothing.
//
//effectgo:errors ErrX
func NoErrResult() int { return 1 } // want `effectgo:errors on NoErrResult checks nothing: it has no error result` NoErrResult:"."

// HasErr has an error result, so its contract is checked: silent.
//
//effectgo:errors ErrX
func HasErr() error { return ErrX } // want HasErr:"."

type T struct{}

// M is a method with no error result.
//
//effectgo:errors ErrX
func (T) M() int { return 0 } // want `effectgo:errors on T.M checks nothing` M:"."

// Serve is the net/http handler shape. It has no error result, so the handler
// directive checks nothing, and the message points to the inner function.
//
//effectgo:handler mapErr
func Serve(w http.ResponseWriter, r *http.Request) {} // want `effectgo:handler on Serve checks nothing: it has no error result; put the directive on the inner function that returns error \(see docs/guide/http-checks.md#nethttp-handlers\)` Serve:"."

// Make is a handler constructor that returns a literal with an error result. The
// directive checks the literal's error set, so it is silent.
//
//effectgo:handler mapErr
func Make() func() error { return func() error { return nil } } // want Make:"."

// MakeNet is a handler constructor whose literal is net/http-shaped: nothing is checked.
//
//effectgo:handler mapErr
func MakeNet() func(http.ResponseWriter, *http.Request) { // want `effectgo:handler on MakeNet checks nothing` MakeNet:"."
	return func(w http.ResponseWriter, r *http.Request) {}
}

// mapErr is the errmap of the handlers above.
//
//effectgo:errmap
func mapErr(err error) error { return err } // want mapErr:"." mapErr:"."
