// Package service holds the contract-annotated functions of the editor example.
package service

import (
	"time"

	"example.com/editor/store"
)

// Stamp is declared pure, but it reaches the clock through time.Now (the
// shipped stdlib stubs). This is a violation.
//
//effectgo:pure
func Stamp(s string) string {
	return s + time.Now().Format(time.RFC3339)
}

// Remember is declared pure, but store.Save has the db effect declared by the
// project stub file. This is a violation.
//
//effectgo:pure
func Remember(s string) error {
	return store.Save(s)
}

// Clean is pure and has no violation.
//
//effectgo:pure
func Clean(s string) string {
	return s + "!"
}
