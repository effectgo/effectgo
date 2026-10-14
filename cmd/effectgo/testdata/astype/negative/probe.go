package astypenegative

import (
	stderrors "errors"

	"example.com/astypenegative/fakeerrors"
)

var ErrBusy = stderrors.New("busy")

type BusyError struct{}

func (*BusyError) Error() string { return "busy" }

type OtherError struct{}

func (*OtherError) Error() string { return "other" }

func source() error { return &BusyError{} }

// Matching another type must not remove BusyError from the false branch.
//
//effectgo:errors ErrBusy
func Mismatch() error {
	err := source()
	if _, ok := stderrors.AsType[*OtherError](err); ok {
		return ErrBusy
	}
	return err
}

// The imported package and generic function have the standard library's names,
// but their implementation does not match errors and must not narrow err.
//
//effectgo:errors ErrBusy
func NonStdlib() error {
	err := source()
	if _, ok := errors.AsType[*BusyError](err); ok {
		return ErrBusy
	}
	return err
}
