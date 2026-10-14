// Package limitsasttype pins a limit of the AST engine: it does not narrow on
// errors.AsType, as it does not narrow on errors.As. The SSA engine gives (none)
// for the same function (see testdata/src/astype, negated).
package limitsasttype

import "errors"

type BusyError struct{}

func (*BusyError) Error() string { return "busy" } // want Error:"."

func busy() error { return &BusyError{} } // want busy:"." "errors of busy: limitsasttype.BusyError"

// negated returns err only when the type test fails. The AST engine still keeps
// BusyError here, which is the limit.
func negated() error { // want negated:"." "errors of negated: limitsasttype.BusyError"
	err := busy()
	if _, ok := errors.AsType[*BusyError](err); !ok {
		return err
	}
	return nil
}
