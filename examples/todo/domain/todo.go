// Package domain holds the Todo model, an in-memory store, and a service
// exposing two API flavors over the same store: idiomatic (T, error) methods
// and Result-returning methods suffixed with R.
package domain

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Todo is a single todo item.
type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

var (
	// ErrNotFound means no todo has the requested id.
	ErrNotFound = errors.New("todo not found")
	// ErrInvalid means the input failed validation.
	ErrInvalid = errors.New("invalid todo")
	// ErrConflict means another todo already has the requested title.
	ErrConflict = errors.New("todo title already taken")
)

const maxTitleLen = 100

// Store is a mutex-protected in-memory map of todos.
type Store struct {
	mu    sync.Mutex
	next  int
	items map[int]Todo
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{next: 1, items: make(map[int]Todo)}
}

func (s *Store) insert(title string) Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Todo{ID: s.next, Title: title}
	s.items[t.ID] = t
	s.next++
	return t
}

func (s *Store) get(id int) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.items[id]
	return t, ok
}

// modify applies fn to a copy of the todo and stores the copy only if fn
// succeeds. The lock is held for the whole read-modify-write.
func (s *Store) modify(id int, fn func(*Todo) error) (Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.items[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	if err := fn(&cur); err != nil {
		return Todo{}, err
	}
	s.items[id] = cur
	return cur, nil
}

func (s *Store) delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	return true
}

func (s *Store) list() []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Todo, 0, len(s.items))
	for _, t := range s.items {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// checkTitle trims the title and validates it. It is shared by both flavors.
func checkTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		return "", ErrInvalid
	}
	if utf8.RuneCountInString(t) > maxTitleLen {
		return "", ErrInvalid
	}
	return t, nil
}

// replace overwrites an existing todo. It reports false if the id is gone,
// which can happen when a delete lands between the read and this write.
func (s *Store) replace(t Todo) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[t.ID]; !ok {
		return false
	}
	s.items[t.ID] = t
	return true
}

// checkTitle returns ErrConflict when another todo (not id) already has title.
func (s *Store) checkTitle(id int, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for oid, o := range s.items {
		if oid != id && o.Title == title {
			return ErrConflict
		}
	}
	return nil
}
