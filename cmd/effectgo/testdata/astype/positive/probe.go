package astypepositive

import "errors"

var ErrBusy = errors.New("busy")

type BusyError struct{}

func (*BusyError) Error() string { return "busy" }

func source() error { return &BusyError{} }

//effectgo:errors ErrBusy
func Modern() error {
	err := source()
	if _, ok := errors.AsType[*BusyError](err); ok {
		return ErrBusy
	}
	return err
}

//effectgo:errors ErrBusy
func ModernNegated() error {
	err := source()
	if _, ok := errors.AsType[*BusyError](err); !ok {
		return err
	}
	return ErrBusy
}

//effectgo:errors ErrBusy
func ModernAssigned() error {
	err := source()
	_, ok := errors.AsType[*BusyError](err)
	if ok {
		return ErrBusy
	}
	return err
}

//effectgo:errors ErrBusy
func Traditional() error {
	err := source()
	var busy *BusyError
	if errors.As(err, &busy) {
		return ErrBusy
	}
	return err
}

//effectgo:errors ErrBusy
func TraditionalNegated() error {
	err := source()
	var busy *BusyError
	if !errors.As(err, &busy) {
		return err
	}
	return ErrBusy
}

//effectgo:errors ErrBusy
func TraditionalAssigned() error {
	err := source()
	var busy *BusyError
	ok := errors.As(err, &busy)
	if ok {
		return ErrBusy
	}
	return err
}
