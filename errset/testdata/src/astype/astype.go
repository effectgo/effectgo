package astype

import (
	"errors"

	"astypeprobe"
)

// BusyError and OtherError are pointer error types, and ErrA is a sentinel.
type BusyError struct{}

func (*BusyError) Error() string { return "busy" } // want Error:"."

type OtherError struct{}

func (*OtherError) Error() string { return "other" } // want Error:"."

var ErrA = errors.New("a")

// ValueError has a value receiver, so ValueError and *ValueError are errors.
type ValueError struct{ Code int }

func (ValueError) Error() string { return "value" } // want Error:"."

// Timeout is an interface target. TimeoutError has its method, BusyError does not.
type Timeout interface {
	error
	Timeout() bool
}

type TimeoutError struct{}

func (TimeoutError) Error() string { return "timeout" } // want Error:"."
func (TimeoutError) Timeout() bool { return true }      // want Timeout:"."

// Wrapper wraps another error through Unwrap, as fmt.Errorf with %w does.
type Wrapper struct{ Err error }

func (w *Wrapper) Error() string { return "wrapper" } // want Error:"."
func (w *Wrapper) Unwrap() error { return w.Err }     // want Unwrap:"." "errors of Wrapper.Unwrap: \\?"

func busy() error      { return &BusyError{} }             // want busy:"." "errors of busy: astype.BusyError"
func other() error     { return &OtherError{} }            // want other:"." "errors of other: astype.OtherError"
func value() error     { return ValueError{} }             // want value:"." "errors of value: astype.ValueError"
func timeout() error   { return TimeoutError{} }           // want timeout:"." "errors of timeout: astype.TimeoutError"
func wrap() error      { return &Wrapper{Err: ErrA} }      // want wrap:"." "errors of wrap: astype.Wrapper"
func probeBusy() error { return &astypeprobe.BusyError{} } // want probeBusy:"." "errors of probeBusy: astypeprobe.BusyError"

// modern maps BusyError to ErrA; the false edge excludes the matched BusyError.
func modern() error { // want modern:"." "errors of modern: astype.ErrA"
	err := busy()
	if _, ok := errors.AsType[*BusyError](err); ok {
		return ErrA
	}
	return err
}

// negated returns err only when the type test fails.
func negated() error { // want negated:"." "errors of negated: \\(none\\)"
	err := busy()
	if _, ok := errors.AsType[*BusyError](err); !ok {
		return err
	}
	return nil
}

// valueTrue returns the value of the matched type, whose variant is the type argument.
func valueTrue() error { // want valueTrue:"." "errors of valueTrue: astype.BusyError"
	err := busy()
	if v, ok := errors.AsType[*BusyError](err); ok {
		return v
	}
	return nil
}

// laterCheck tests ok after other statements. Its false edge returns err, narrowed.
func laterCheck() error { // want laterCheck:"." "errors of laterCheck: \\(none\\)"
	err := busy()
	v, ok := errors.AsType[*BusyError](err)
	_ = v
	if ok {
		return nil
	}
	return err
}

// trueOther returns on the true edge of a test that does not match the value.
func trueOther() error { // want trueOther:"." "errors of trueOther: \\(none\\)"
	err := busy()
	if _, ok := errors.AsType[*OtherError](err); ok {
		return err
	}
	return nil
}

// valueType is a value target: the test matches ValueError, so the false edge drops it.
func valueType() error { // want valueType:"." "errors of valueType: \\(none\\)"
	err := value()
	if _, ok := errors.AsType[ValueError](err); ok {
		return nil
	}
	return err
}

// valueKept is a value target that BusyError does not match, so the false edge keeps it.
func valueKept() error { // want valueKept:"." "errors of valueKept: astype.BusyError"
	err := busy()
	if _, ok := errors.AsType[ValueError](err); ok {
		return nil
	}
	return err
}

// ifaceDrop is an interface target that TimeoutError implements, so the false edge drops it.
func ifaceDrop() error { // want ifaceDrop:"." "errors of ifaceDrop: \\(none\\)"
	err := timeout()
	if _, ok := errors.AsType[Timeout](err); ok {
		return nil
	}
	return err
}

// ifaceKeep is an interface target that BusyError does not implement, so it keeps it.
func ifaceKeep() error { // want ifaceKeep:"." "errors of ifaceKeep: astype.BusyError"
	err := busy()
	if _, ok := errors.AsType[Timeout](err); ok {
		return nil
	}
	return err
}

// wrapFalse: a wrapper may hold the target, because errors.As unwraps, so the false edge keeps it.
func wrapFalse() error { // want wrapFalse:"." "errors of wrapFalse: astype.Wrapper"
	err := wrap()
	if _, ok := errors.AsType[*BusyError](err); ok {
		return nil
	}
	return err
}

// wrapTrue keeps the wrapper on the true edge too.
func wrapTrue() error { // want wrapTrue:"." "errors of wrapTrue: astype.Wrapper"
	err := wrap()
	if _, ok := errors.AsType[*BusyError](err); ok {
		return err
	}
	return nil
}

// isBusy returns the ok result itself, so its fact is the test on BusyError.
func isBusy(err error) bool { // want isBusy:"errset-test: errors.As astype.BusyError" isBusy:"errors=\\(none\\)"
	_, ok := errors.AsType[*BusyError](err)
	return ok
}

// isOther tests OtherError through an if statement that returns constants.
func isOther(err error) bool { // want isOther:"errset-test: errors.As astype.OtherError" isOther:"errors=\\(none\\)"
	if _, ok := errors.AsType[*OtherError](err); ok {
		return true
	}
	return false
}

// predBusy uses the predicate on the false edge of its negation.
func predBusy() error { // want predBusy:"." "errors of predBusy: \\(none\\)"
	err := busy()
	if !isBusy(err) {
		return err
	}
	return nil
}

// predOther: isOther does not match BusyError, so the false edge keeps it.
func predOther() error { // want predOther:"." "errors of predOther: astype.BusyError"
	err := busy()
	if isOther(err) {
		return nil
	}
	return err
}

// probeGuard uses a predicate of another package, which tests astypeprobe.BusyError.
func probeGuard() error { // want probeGuard:"." "errors of probeGuard: \\(none\\)"
	err := probeBusy()
	if !astypeprobe.IsBusy(err) {
		return err
	}
	return nil
}

// probeOther: the other package's OtherError test does not match astypeprobe.BusyError.
func probeOther() error { // want probeOther:"." "errors of probeOther: astypeprobe.BusyError"
	err := probeBusy()
	if astypeprobe.IsOther(err) {
		return nil
	}
	return err
}

// A test narrows the SSA value passed to it, not a later assignment to err.
func reassignedSubject() error { // want reassignedSubject:"." "errors of reassignedSubject: astype.OtherError"
	err := busy()
	_, ok := errors.AsType[*BusyError](err)
	err = other()
	if !ok {
		return err
	}
	return nil
}

// A replaced boolean no longer carries the AsType predicate.
func reassignedFlag(flag bool) error { // want reassignedFlag:"." "errors of reassignedFlag: astype.BusyError"
	err := busy()
	_, ok := errors.AsType[*BusyError](err)
	ok = flag
	if !ok {
		return err
	}
	return nil
}

// Only the matching member is removed from a merged input set.
func mixedInput(flag bool) error { // want mixedInput:"." "errors of mixedInput: astype.OtherError"
	err := busy()
	if flag {
		err = other()
	}
	if _, ok := errors.AsType[*BusyError](err); ok {
		return nil
	}
	return err
}

func errorTarget() error { // want errorTarget:"." "errors of errorTarget: \\(none\\)"
	err := busy()
	if _, ok := errors.AsType[error](err); ok {
		return nil
	}
	return err
}

// An early false return is not equivalent to testing the type on every path.
func filteredBusy(err error) bool { // want filteredBusy:"errors=\\(none\\)"
	if err != nil {
		return false
	}
	_, ok := errors.AsType[*BusyError](err)
	return ok
}

func filteredOther(err error) bool { // want filteredOther:"errors=\\(none\\)"
	if err != nil {
		return false
	}
	if _, ok := errors.AsType[*OtherError](err); ok {
		return true
	}
	return false
}

func partialPredicate() error { // want partialPredicate:"." "errors of partialPredicate: astype.BusyError"
	err := busy()
	if filteredBusy(err) {
		return nil
	}
	return err
}

func partialBranchPredicate() error { // want partialBranchPredicate:"." "errors of partialBranchPredicate: astype.OtherError"
	err := other()
	if filteredOther(err) {
		return nil
	}
	return err
}

// isBusyElse is the else spelling of the isBusy test: the same predicate.
func isBusyElse(err error) bool { // want isBusyElse:"errset-test: errors.As astype.BusyError" isBusyElse:"errors=\\(none\\)"
	if _, ok := errors.AsType[*BusyError](err); ok {
		return true
	} else {
		return false
	}
}

// elseGuard narrows by the else spelling of the predicate.
func elseGuard() error { // want elseGuard:"." "errors of elseGuard: \\(none\\)"
	err := busy()
	if isBusyElse(err) {
		return nil
	}
	return err
}

// The test is of another type, so BusyError stays in the false branch and the
// contract is violated: this is the diagnostic that a mismatched test must keep.
//
//effectgo:errors ErrA
func mismatchContract() error { // want "mismatchContract may return astype.BusyError" "errors of mismatchContract: .*astype.BusyError" mismatchContract:"."
	err := busy()
	if _, ok := errors.AsType[*OtherError](err); ok {
		return ErrA
	}
	return err
}
