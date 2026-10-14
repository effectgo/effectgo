// Package predcontract checks the contracts that call the predicates of errors.As
// and errors.AsType with a constant false return before their test. Both error-set
// engines report the callers: the SSA engine exports no test for these predicates,
// and the AST engine does not narrow on predicates at all.
package predcontract

import "errors"

var ErrA = errors.New("a")

type BusyError struct{}

func (*BusyError) Error() string { return "busy" } // want Error:"."

// disabled makes IsBusyAs and IsBusyType return false whatever err is.
var disabled bool

// source returns a BusyError.
func source() error { return &BusyError{} } // want source:"errors=predcontract.BusyError"

// IsBusyAs returns false before its errors.As test.
func IsBusyAs(err error) bool { // want IsBusyAs:"errors=\\(none\\)"
	if disabled {
		return false
	}
	var b *BusyError
	return errors.As(err, &b)
}

// IsBusyType returns false before its errors.AsType test.
func IsBusyType(err error) bool { // want IsBusyType:"errors=\\(none\\)"
	if disabled {
		return false
	}
	_, ok := errors.AsType[*BusyError](err)
	return ok
}

// CallerAs is reported: the false edge of IsBusyAs can still hold a BusyError.
//
//effectgo:errors ErrA
func CallerAs() error { // want CallerAs:"errors=predcontract\\.BusyError \\| predcontract\\.ErrA" "CallerAs may return predcontract.BusyError"
	err := source()
	if IsBusyAs(err) {
		return ErrA
	}
	return err
}

// CallerType is reported: the false edge of IsBusyType can still hold a BusyError.
//
//effectgo:errors ErrA
func CallerType() error { // want CallerType:"errors=predcontract\\.BusyError \\| predcontract\\.ErrA" "CallerType may return predcontract.BusyError"
	err := source()
	if IsBusyType(err) {
		return ErrA
	}
	return err
}
