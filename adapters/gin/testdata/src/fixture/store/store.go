// Package store holds the sentinels and the operations of the fixture.
package store

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid")
	ErrLocked   = errors.New("locked")
)

type Item struct{ ID int }

type Store struct{}

func (s *Store) Get(id int) (Item, error) {
	if id == 0 {
		return Item{}, ErrNotFound
	}
	if id < 0 {
		return Item{}, ErrInvalid
	}
	return Item{ID: id}, nil
}

func (s *Store) Create(id int) error {
	if id == 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) Lock() error { return ErrLocked }

// Fail returns a fresh error (new@), which no errors.Is can match.
func (s *Store) Fail() error { return errors.New("boom") }

type ValidationError struct{}

func (*ValidationError) Error() string { return "invalid" }

func (s *Store) Validate(id int) error {
	if id < 0 {
		return &ValidationError{}
	}
	return ErrNotFound
}
