// Package sigs exercises the signature analyzer. Each expectation names the
// function and its expected Effect<A, E, R> line.
package sigs

import (
	"errors"
	"net/http"
	"os"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
)

type Todo struct{ Title string }

type Store struct{}

func Nop() {} // want `Nop:\s+Effect<void, never, \{\}>`

func Pure(x int) int { return x } // want `Pure:\s+Effect<int, never, \{\}>`

func Find(id int) (Todo, error) { // want `Find:\s+Effect<Todo, ErrNotFound, \{\}>`
	if id < 0 {
		return Todo{}, ErrNotFound
	}
	return Todo{}, nil
}

// Fetch: the error comes from a stdlib call, which is opaque.
func Fetch(url string) ([]byte, error) { // want `Fetch:\s+Effect<\[\]byte, \?, \{net\}>`
	_, err := http.Get(url)
	return nil, err
}

func Env() string { return os.Getenv("HOME") } // want `Env:\s+Effect<string, never, \{env\}>`

func Pair() (int, string, error) { // want `Pair:\s+Effect<\(int, string\), ErrInvalid, \{\}>`
	return 0, "", ErrInvalid
}

// modify passes its error through from a call of its own func parameter.
func (s *Store) modify(id int, fn func(*Todo) error) (Todo, error) { // want `Store\.modify:\s+Effect<Todo, ErrNotFound \| param:1, \{param:1\}>`
	if id < 0 {
		return Todo{}, ErrNotFound
	}
	var t Todo
	if err := fn(&t); err != nil {
		return Todo{}, err
	}
	return t, nil
}

// Handler returns a handler literal; the signature describes the handler.
func Handler(s *Store) func(id int) (Todo, error) { // want `Handler:\s+Effect<Todo, ErrNotFound, \{\}>`
	return func(id int) (Todo, error) {
		return Todo{}, ErrNotFound
	}
}

// Clock violates its contract. The effects analyzer reports that, but the
// signature analyzer must not: only its own diagnostics are expected here.
//
//effectgo:pure
func Clock() int64 { // want `Clock:\s+Effect<int64, never, \{time\}>`
	return time.Now().Unix()
}
