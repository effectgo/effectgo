// Package predtail has a predicate in the straight-line form of errors.As, and a
// caller that narrows by it.
package predtail

import "errors"

// ErrA is the error the caller returns when the predicate holds.
var ErrA = errors.New("a")

// BusyError is the typed error the predicate tests.
type BusyError struct{}

func (*BusyError) Error() string { return "busy" }

func source() error { return &BusyError{} }

// IsBusy has the straight-line form, so it exports its test.
func IsBusy(err error) bool {
	var b *BusyError
	if errors.As(err, &b) {
		return true
	}
	return false
}

// CallerBusy is not reported: the false edge of IsBusy removes BusyError.
//
//effectgo:errors ErrA
func CallerBusy() error {
	err := source()
	if IsBusy(err) {
		return ErrA
	}
	return err
}
