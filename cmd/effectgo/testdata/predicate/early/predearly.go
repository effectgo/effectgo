// Package predearly has a predicate whose constant false return comes before its
// errors.As test, and a caller that narrows by it.
package predearly

import "errors"

// ErrA is the error the caller returns when the predicate holds.
var ErrA = errors.New("a")

// BusyError is the typed error the predicate tests.
type BusyError struct{}

func (*BusyError) Error() string { return "busy" }

// disabled makes IsBusy return false whatever err is.
var disabled bool

func source() error { return &BusyError{} }

// IsBusy returns false before its errors.As test, so it exports no test.
func IsBusy(err error) bool {
	if disabled {
		return false
	}
	var b *BusyError
	return errors.As(err, &b)
}

// CallerBusy is reported: the false edge of IsBusy can still hold a BusyError.
//
//effectgo:errors ErrA
func CallerBusy() error {
	err := source()
	if IsBusy(err) {
		return ErrA
	}
	return err
}
