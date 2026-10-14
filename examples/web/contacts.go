package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Contact is the response of GET /api/contacts/{id} and POST /api/contact.
type Contact struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ContactInput is the request body of POST /api/contact.
type ContactInput struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Store is an in-memory contact table.
type Store struct {
	mu       sync.Mutex
	contacts map[int64]Contact
}

// NewStore returns a store seeded with one contact (id 1).
func NewStore() *Store {
	return &Store{contacts: map[int64]Contact{
		1: {ID: 1, Name: "Ada", Email: "ada@example.com"},
	}}
}

// GetContactInput is the path of GET /api/contacts/{id}.
type GetContactInput struct {
	ID int64 `path:"id" json:"id"`
}

// GetContact returns the contact with the given id, or ContactNotFound.
func (s *Store) GetContact(ctx context.Context, in GetContactInput) (Contact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.contacts[in.ID]
	if !ok {
		return Contact{}, ContactNotFound{Detail: fmt.Sprintf("contact %d not found", in.ID)}
	}
	return c, nil
}

// UpdateContact validates the input, then replaces the stored contact with
// the same id. It returns ValidationError for a bad name, and ContactNotFound
// when the id is unknown.
func (s *Store) UpdateContact(ctx context.Context, in ContactInput) (Contact, error) {
	if err := validateContact(in); err != nil {
		return Contact{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.contacts[in.ID]; !ok {
		return Contact{}, ContactNotFound{Detail: fmt.Sprintf("contact %d not found", in.ID)}
	}
	c := Contact{ID: in.ID, Name: in.Name, Email: in.Email}
	s.contacts[in.ID] = c
	return c, nil
}

func validateContact(in ContactInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return ValidationError{Detail: "name must not be empty"}
	}
	if strings.ContainsAny(in.Name, "\r\n") {
		return ValidationError{Detail: "name must be a single line"}
	}
	return nil
}
