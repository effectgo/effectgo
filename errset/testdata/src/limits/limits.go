package limits

import (
	"domain"
	"errors"
)

type holder struct{ err error }

func (h *holder) get() error { return domain.ErrNotFound } // want get:"." "errors of holder.get: domain.ErrNotFound"

type doer interface{ Do() error }

// Flow-insensitive: the nil reassignment does not remove ErrNotFound (false positive).
func overwritten() error { // want overwritten:"." "errors of overwritten: domain.ErrNotFound"
	err := domain.ErrNotFound
	if errors.Is(err, domain.ErrNotFound) {
		err = nil
	}
	return err
}

func viaInterface(d doer) error { // want viaInterface:"." "errors of viaInterface: \\?"
	return d.Do()
}

func viaField(h *holder) error { // want viaField:"." "errors of viaField: \\?"
	return h.err
}

func viaFuncValue(f func() error) error { // want viaFuncValue:"." "errors of viaFuncValue: param:0"
	return f()
}

func viaMethodValue(h *holder) error { // want viaMethodValue:"." "errors of viaMethodValue: \\?"
	f := h.get
	return f()
}

func wrapT[E error](e E) error { // want wrapT:"." "errors of wrapT: \\?"
	return e
}

func genErr[T any](v T) error { // want genErr:"." "errors of genErr: domain.ErrInvalid"
	return domain.ErrInvalid
}

func callGen() error { // want callGen:"." "errors of callGen: domain.ErrInvalid"
	return genErr(1)
}
