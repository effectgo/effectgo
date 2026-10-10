// Package status exercises HTTP status inference for errmaps (errset/status.go).
package status

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound = errors.New("not found")
	ErrDup      = errors.New("duplicate")
	ErrGone     = errors.New("gone")
)

// StatusError is the interface that ToStatus passes through with errors.As.
type StatusError interface {
	error
	StatusCode() int
}

// ConstErr has a constant StatusCode: 409 is inferred from the body. The stub
// file gives it 418, which inference overrides.
type ConstErr struct{}

func (ConstErr) Error() string   { return "conflict" } // want Error:"."
func (ConstErr) StatusCode() int { return 409 }        // want StatusCode:"." StatusCode:"."

// StubErr's status depends on its field, so it has no constant. The stub file
// gives it 422.
type StubErr struct{ Code int }

func (e StubErr) Error() string { return "stub" } // want Error:"."
func (e StubErr) StatusCode() int { // want StatusCode:"."
	if e.Code == 0 {
		return 500
	}
	return e.Code
}

// HTTPErr is shaped like fuego.HTTPError: a Status field and a dynamic StatusCode.
type HTTPErr struct {
	Status int
	Detail string
}

func (e HTTPErr) Error() string { return e.Detail } // want Error:"."
func (e HTTPErr) StatusCode() int { // want StatusCode:"."
	if e.Status == 0 {
		return 500
	}
	return e.Status
}

// Plain has no status method: Fuego sends it as 500.
type Plain struct{}

func (Plain) Error() string { return "plain" } // want Error:"."

// Opaque returns an error whose type is not visible; the stub file gives it
// the variant iface:status.StatusError.
func Opaque() error { return errors.New("opaque") } // want Opaque:"."

// ToStatus maps ErrNotFound to HTTPErr{Status: 409}, and ErrDup to an HTTPErr
// without a status (unknown). A StatusError passes through with its own status.
//
//effectgo:errmap
func ToStatus(err error) error { // want ToStatus:"." ToStatus:"."
	var se StatusError
	if errors.As(err, &se) {
		return err
	}
	if errors.Is(err, ErrNotFound) {
		return HTTPErr{Status: 409, Detail: "not found"}
	}
	if errors.Is(err, ErrDup) {
		return HTTPErr{Detail: "duplicate"}
	}
	return err
}

//effectgo:handler ToStatus
func HConst() func() (int, error) { return func() (int, error) { return 0, ConstErr{} } } // want HConst:"." "statuses of HConst: 409 status.ConstErr"

//effectgo:handler ToStatus
func HStub() func() (int, error) { return func() (int, error) { return 0, StubErr{} } } // want HStub:"." "statuses of HStub: 422 status.StubErr"

//effectgo:handler ToStatus
func HNotFound() func() (int, error) { return func() (int, error) { return 0, ErrNotFound } } // want HNotFound:"." "statuses of HNotFound: 409 status.ErrNotFound"

//effectgo:handler ToStatus
func HDup() func() (int, error) { return func() (int, error) { return 0, ErrDup } } // want HDup:"." "statuses of HDup: default status.ErrDup"

//effectgo:handler ToStatus
func HGone() func() (int, error) { return func() (int, error) { return 0, ErrGone } } // want HGone:"." "may return status.ErrGone, which errmap status.ToStatus does not map" "statuses of HGone: 500 status.ErrGone"

//effectgo:handler ToStatus
func HPlain() func() (int, error) { return func() (int, error) { return 0, Plain{} } } // want HPlain:"." "may return status.Plain, which errmap status.ToStatus does not map" "statuses of HPlain: 500 status.Plain"

//effectgo:handler ToStatus
func HOpaque() func() (int, error) { return func() (int, error) { return 0, Opaque() } } // want HOpaque:"." "statuses of HOpaque: default some status.StatusError"

//effectgo:handler ToStatus
func HUnclassified() func() (int, error) { // want HUnclassified:"." "unclassified error" "statuses of HUnclassified: 500"
	return func() (int, error) { return 0, fmt.Errorf(unclassifiedFormat, 1) }
}

// unclassifiedFormat is not a constant, so the error it formats is opaque.
var unclassifiedFormat = "x %v"
