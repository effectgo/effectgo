package domain

import "errors"

var (
	ErrNotFound = errors.New("todo not found")
	ErrInvalid  = errors.New("invalid todo")
	ErrConflict = errors.New("title already taken")
)

// ValidationError is an error type with a status method.
type ValidationError struct{ Field string }

func (e *ValidationError) Error() string   { return "invalid " + e.Field }
func (e *ValidationError) StatusCode() int { return 422 }

// Todo is a model type; it does not implement error.
type Todo struct {
	ID    int
	Title string
}

type Store struct{ items map[int]Todo }

// modify is where the store-level uniqueness check lives.
func (s *Store) modify(id int, title string) (Todo, error) {
	for oid, o := range s.items {
		if oid != id && o.Title == title {
			return Todo{}, ErrConflict
		}
	}
	if _, ok := s.items[id]; !ok {
		return Todo{}, ErrNotFound
	}
	return s.items[id], nil
}

type Service struct{ store *Store }

// Update reaches ErrConflict through Store.modify.
func (s *Service) Update(id int, title string) (Todo, error) {
	if title == "" {
		return Todo{}, ErrInvalid
	}
	return s.store.modify(id, title)
}

// Get reaches ErrNotFound directly.
func (s *Service) Get(id int) (Todo, error) {
	if t, ok := s.store.items[id]; ok {
		return t, nil
	}
	return Todo{}, ErrNotFound
}

// Check returns a validation error type.
func (s *Service) Check(title string) error {
	if title == "" {
		return &ValidationError{Field: "title"}
	}
	return nil
}
