// Package limitsast pins a known limit of the AST engine (the default). It is
// flow-insensitive: the nil reassignment does not remove ErrNotFound. The SSA
// engine gives "(none)" for the same function (see ssaflow.guardReset).
package limitsast

import (
	"domain"
	"errors"
)

// Flow-insensitive: the nil reassignment does not remove ErrNotFound (false positive).
func overwritten() error { // want overwritten:"." "errors of overwritten: domain.ErrNotFound"
	err := domain.ErrNotFound
	if errors.Is(err, domain.ErrNotFound) {
		err = nil
	}
	return err
}
