package basic

import (
	"domain"
	"errors"
	"fmt"
	"strconv"
)

// LocalErr is an error type declared in this package.
type LocalErr struct{}

func (LocalErr) Error() string { return "local" } // want Error:"."

func sentinel() error { return domain.ErrNotFound } // want sentinel:"."

//effectgo:errors LocalErr
func errTypePtr() error { return &LocalErr{} } // want errTypePtr:"."

func errTypeValue() error { return LocalErr{} } // want errTypeValue:"."

func errTypeNew() error { return new(LocalErr) } // want errTypeNew:"."

func wrapW() error { return fmt.Errorf("ctx: %w", domain.ErrInvalid) } // want wrapW:"."

func wrapV() error { return fmt.Errorf("ctx: %v", domain.ErrInvalid) } // want wrapV:"."

func joined() error { return errors.Join(domain.ErrNotFound, LocalErr{}) } // want joined:"."

func localFlow(n int) error { // want localFlow:"."
	var err error
	if n > 0 {
		err = domain.ErrNotFound
	} else {
		err = LocalErr{}
	}
	return err
}

func shortDecl(s string) error { // want shortDecl:"."
	_, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	if err := sentinelFor(s); err != nil {
		return err
	}
	return nil
}

func sentinelFor(s string) error { // want sentinelFor:"."
	if s == "" {
		return domain.ErrInvalid
	}
	return nil
}

type store struct{}

func (store) find() error { return domain.ErrNotFound } // want find:"."

func service(s store) error { // want service:"."
	if err := s.find(); err != nil {
		return err
	}
	return nil
}

func handler() error { return service(store{}) } // want handler:"."

//effectgo:errors domain.ErrInvalid
func contractChain() error { return service(store{}) } // want "contractChain may return domain.ErrNotFound \\(via service → store.find\\), not in its //effectgo:errors contract" contractChain:"."

func nilOnly() error { return nil } // want nilOnly:"."
