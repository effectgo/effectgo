// Package ifacemap holds errmaps with interface targets. A dependency of ifaceweb.
package ifacemap

import (
	"errors"

	"ifacedef"
)

// Map covers ErrNotFound by key and every ifacedef.Status by errors.As.
//
//effectgo:errmap
func Map(err error) error {
	var st ifacedef.Status
	if errors.As(err, &st) {
		return err
	}
	if errors.Is(err, ifacedef.ErrNotFound) {
		return nil
	}
	return err
}

// Strict covers only ErrNotFound.
//
//effectgo:errmap
func Strict(err error) error {
	if errors.Is(err, ifacedef.ErrNotFound) {
		return nil
	}
	return err
}

// Loose covers ErrNotFound, and every error whose type has a StatusCode method
// (the method set of ifacedef.Status includes it, so some ifacedef.Status is covered).
//
//effectgo:errmap
func Loose(err error) error {
	var sc interface{ StatusCode() int }
	if errors.As(err, &sc) {
		return err
	}
	if errors.Is(err, ifacedef.ErrNotFound) {
		return nil
	}
	return err
}
