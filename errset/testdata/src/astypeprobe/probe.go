// Package astypeprobe has predicates that test errors.AsType. The package that
// imports it reads their facts.
package astypeprobe

import "errors"

// BusyError and OtherError are pointer error types.
type BusyError struct{}

func (*BusyError) Error() string { return "busy" }

type OtherError struct{}

func (*OtherError) Error() string { return "other" }

// IsBusy returns the ok result of errors.AsType on *BusyError.
func IsBusy(err error) bool {
	_, ok := errors.AsType[*BusyError](err)
	return ok
}

// IsOther tests *OtherError through an if statement that returns constants.
func IsOther(err error) bool {
	if _, ok := errors.AsType[*OtherError](err); ok {
		return true
	}
	return false
}
