package deferflow

import (
	"errors"
	"fmt"
)

var (
	ErrStorage = errors.New("storage")
	ErrRaw     = errors.New("raw")
	ErrA       = errors.New("a")
	ErrB       = errors.New("b")
)

func step() error { return ErrRaw } // want step:"errset: errors=deferflow.ErrRaw" "errors of step: deferflow.ErrRaw"

// Named is the repro of the named result captured by a deferred closure that
// only reads it: the set is the wrapped return, not the raw step error.
func Named() (err error) { // want Named:"." "errors of Named: deferflow.ErrStorage"
	defer func() {
		if err != nil {
			_ = err
		}
	}()
	if err = step(); err != nil {
		return fmt.Errorf("%w: %v", ErrStorage, err)
	}
	return nil
}

// Tx and DB stand for database/sql: Begin and Rollback are opaque.
type Tx interface{ Rollback() error }

type DB interface {
	Begin() (Tx, error)
}

type Store struct{ db DB }

// Hold is the transaction shape: the raw Begin error is not returned.
func (s *Store) Hold() (err error) { // want Hold:"errset: errors=deferflow.ErrStorage" "errors of Store.Hold: deferflow.ErrStorage"
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorage, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = step(); err != nil {
		return fmt.Errorf("%w: %v", ErrStorage, err)
	}
	return nil
}

// Wrapped writes the result in its defer, wrapping with %w: the wrap keeps
// the variant of the value it reads.
func Wrapped() (err error) { // want Wrapped:"." "errors of Wrapped: deferflow.ErrRaw"
	defer func() {
		if err != nil {
			err = fmt.Errorf("wrap: %w", err)
		}
	}()
	return step()
}

// WrappedV writes the result with %v: the write is opaque.
func WrappedV() (err error) { // want WrappedV:"." "errors of WrappedV: deferflow.ErrA | \\?"
	defer func() {
		if err != nil {
			err = fmt.Errorf("wrap: %v", err)
		}
	}()
	return ErrA
}

// Overwrite's defer writes another sentinel: both are returned.
func Overwrite() (err error) { // want Overwrite:"." "errors of Overwrite: deferflow.ErrA | deferflow.ErrB"
	defer func() {
		if err != nil {
			err = ErrB
		}
	}()
	return ErrA
}

// Naked returns the raw error through a naked return.
func Naked() (err error) { // want Naked:"." "errors of Naked: deferflow.ErrRaw"
	defer func() {
		if err != nil {
			_ = err
		}
	}()
	if err = step(); err != nil {
		return
	}
	return nil
}

func fill(p *error) { *p = ErrB } // want fill:"."

// Escape passes the address of the result to a deferred call, which may write
// it: the opaque error is added.
func Escape() (err error) { // want Escape:"." "errors of Escape: deferflow.ErrA | \\?"
	defer fill(&err)
	return ErrA
}

// Recovers: a recovered panic returns the variable as it was, so every stored
// value is returned.
func Recovers() (err error) { // want Recovers:"." "errors of Recovers: deferflow.ErrA | deferflow.ErrRaw"
	defer func() {
		if r := recover(); r != nil {
			_ = r
		}
	}()
	if err = step(); err != nil {
		return ErrA
	}
	return nil
}

// Stored: a reassignment that is overwritten by the return, with a read-only
// defer: only the return's value.
func Stored() (err error) { // want Stored:"." "errors of Stored: deferflow.ErrA"
	defer func() {
		_ = err
	}()
	err = ErrRaw
	err = ErrA
	return err
}

// ReturnLoad returns the variable itself: the set is the value that reaches
// the return, not the union of the stores (ErrA is stored after it).
func ReturnLoad() (err error) { // want ReturnLoad:"." "errors of ReturnLoad: deferflow.ErrRaw"
	defer func() {
		if err != nil {
			_ = err
		}
	}()
	if err = step(); err != nil {
		return err
	}
	err = ErrA
	return nil
}
