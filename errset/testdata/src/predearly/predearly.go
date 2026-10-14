// Package predearly pins the predicate rule of errors.As and errors.AsType on the
// forms that the SSA engine exports. A predicate whose constant false return comes
// before its test exports no test: the false edge of a call would remove the target,
// but the predicate can return false for a target. IsBusyAs and IsBusyType are such
// predicates, so CallerAs and CallerType keep BusyError on the false edge and are
// reported. IsBusyTail has the straight-line form, which still exports its test, so
// CallerTail is narrowed by it.
package predearly

import "errors"

var ErrA = errors.New("a")

type BusyError struct{}

func (*BusyError) Error() string { return "busy" } // want Error:"."

// source returns a BusyError.
func source() error { return &BusyError{} } // want source:"." "errors of source: predearly.BusyError"

// disabled makes IsBusyAs and IsBusyType return false whatever err is.
var disabled bool

// IsBusyAs returns false before its test, so it exports no test.
func IsBusyAs(err error) bool { // want IsBusyAs:"errors=\\(none\\)"
	if disabled {
		return false
	}
	var b *BusyError
	return errors.As(err, &b)
}

// IsBusyType is IsBusyAs with errors.AsType, which exports no test either.
func IsBusyType(err error) bool { // want IsBusyType:"errors=\\(none\\)"
	if disabled {
		return false
	}
	_, ok := errors.AsType[*BusyError](err)
	return ok
}

// IsBusyTail is the straight-line form of errors.As: it exports its test.
func IsBusyTail(err error) bool { // want IsBusyTail:"errset-test: errors.As predearly.BusyError" IsBusyTail:"errors=\\(none\\)"
	var b *BusyError
	if errors.As(err, &b) {
		return true
	}
	return false
}

// CallerAs is reported: the false edge of IsBusyAs keeps BusyError.
//
//effectgo:errors ErrA
func CallerAs() error { // want CallerAs:"." "CallerAs may return predearly.BusyError" "errors of CallerAs: predearly.BusyError \\| predearly.ErrA$"
	err := source()
	if IsBusyAs(err) {
		return ErrA
	}
	return err
}

// CallerTail is not reported: the false edge of IsBusyTail removes BusyError.
//
//effectgo:errors ErrA
func CallerTail() error { // want CallerTail:"." "errors of CallerTail: predearly.ErrA$"
	err := source()
	if IsBusyTail(err) {
		return ErrA
	}
	return err
}

// CallerType is reported: IsBusyType exports no test, so BusyError stays on the false edge.
//
//effectgo:errors ErrA
func CallerType() error { // want "CallerType may return predearly.BusyError" "errors of CallerType: .*predearly.BusyError" CallerType:"."
	err := source()
	if IsBusyType(err) {
		return ErrA
	}
	return err
}
