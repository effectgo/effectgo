package bad

import "errors"

var (
	ErrMissing = errors.New("missing")
	ErrBusy    = errors.New("busy")
)

//effectgo:errors ErrMissing
func Lookup() error {
	return ErrBusy
}
