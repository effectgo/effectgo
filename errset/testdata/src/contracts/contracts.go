package contracts

import (
	"domain"
	"errors"
)

var ErrOwn = errors.New("own")

//effectgo:errors ErrOwn, domain.ErrNotFound
func okContract(n int) error { // want okContract:"."
	if n > 0 {
		return ErrOwn
	}
	return domain.ErrNotFound
}

// Declared but never returned: no report.
//
//effectgo:errors ErrOwn, domain.ErrConflict
func overDeclared() error { return ErrOwn } // want overDeclared:"."

//effectgo:errors ErrOwn
func violated() error { return domain.ErrInvalid } // want "violated may return domain.ErrInvalid, not in its //effectgo:errors contract" violated:"."

//effectgo:errors Missing
func unresolved() error { return nil } // want "effectgo:errors: cannot resolve \"Missing\"" unresolved:"."

// No name of the contract resolves: the contract would be empty only because
// of the failed names, so the real variant is not reported as a violation too.
// A partly resolved contract is still checked (nameMismatch below).
//
//effectgo:errors Missing, Gone
func unresolvedAll() error { return domain.ErrInvalid } // want "cannot resolve \"Missing\"" "cannot resolve \"Gone\"" unresolvedAll:"."

//effectgo:errors ?
func opaqueAllowed(e error) error { return e } // want opaqueAllowed:"."

//effectgo:errors ErrOwn
func opaqueViolated() error { return errors.New("x") } // want "opaqueViolated may return new@contracts.opaqueViolated, not in its //effectgo:errors contract" opaqueViolated:"."

//effectgo:errors domain.Nope, domain.ErrInvalid
func nameMismatch() error { return domain.ErrNotFound } // want "cannot resolve \"domain.Nope\"" "nameMismatch may return domain.ErrNotFound" nameMismatch:"."

//effectgo:bogus
func unknownDirective() {} // want "unknown directive //effectgo:bogus" unknownDirective:"."
