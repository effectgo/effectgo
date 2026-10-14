package astypepositive

import stderrors "errors"

//effectgo:errors ErrBusy
func ModernAliased() error {
	err := source()
	if _, ok := stderrors.AsType[*BusyError](err); ok {
		return ErrBusy
	}
	return err
}
