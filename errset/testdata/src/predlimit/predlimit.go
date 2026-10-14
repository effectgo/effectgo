// Package predlimit pins a limit of the predicate rule that errors.As has. A
// predicate whose constant false return comes before its test exports the test,
// so the false edge of a call removes the target, although the predicate can
// return false for a target. IsBusyAs is such a predicate, and CallerAs is not
// reported although its false edge still holds a BusyError. IsBusyType has the
// same shape with errors.AsType, which exports nothing, so CallerType is
// reported. The errors.As rule is unchanged.
package predlimit

import "errors"

var ErrA = errors.New("a")

type BusyError struct{}

func (*BusyError) Error() string { return "busy" } // want Error:"."

// source returns a BusyError.
func source() error { return &BusyError{} } // want source:"." "errors of source: predlimit.BusyError"

// disabled makes IsBusyAs and IsBusyType return false whatever err is.
var disabled bool

// IsBusyAs returns false before its test, and exports the test anyway.
func IsBusyAs(err error) bool { // want IsBusyAs:"errset-test: errors.As predlimit.BusyError" IsBusyAs:"errors=\\(none\\)"
	if disabled {
		return false
	}
	var b *BusyError
	return errors.As(err, &b)
}

// IsBusyType is IsBusyAs with errors.AsType, which the straight-line rule does not export.
func IsBusyType(err error) bool { // want IsBusyType:"errors=\\(none\\)"
	if disabled {
		return false
	}
	_, ok := errors.AsType[*BusyError](err)
	return ok
}

// CallerAs is not reported: the false edge of IsBusyAs removes BusyError.
//
//effectgo:errors ErrA
func CallerAs() error { // want CallerAs:"." "errors of CallerAs: predlimit.ErrA$"
	err := source()
	if IsBusyAs(err) {
		return ErrA
	}
	return err
}

// CallerType is reported: IsBusyType exports no test, so BusyError stays on the false edge.
//
//effectgo:errors ErrA
func CallerType() error { // want "CallerType may return predlimit.BusyError" "errors of CallerType: .*predlimit.BusyError" CallerType:"."
	err := source()
	if IsBusyType(err) {
		return ErrA
	}
	return err
}
