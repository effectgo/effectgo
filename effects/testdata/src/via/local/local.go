// Package local holds concrete writer types, kept apart from package via, whose
// NetWriter contract their Write methods would violate.
package local

import (
	"fmt"
)

// LocalWriter is a concrete writer whose Write prints: the call site sees
// through the concrete type, so the stdio effect is charged.
type LocalWriter struct{}

func (LocalWriter) Write(p []byte) (int, error) { // want Write:`stdio`
	fmt.Println(string(p))
	return len(p), nil
}

// ToLocal passes a concrete type whose Write is stdio.
//
//effectgo:pure
func ToLocal(n int) { // want ToLocal:`stdio` `ToLocal is declared //effectgo:pure but has effect stdio via ToLocal → fmt\.Fprintf → LocalWriter\.Write → fmt\.Println`
	fmt.Fprintf(LocalWriter{}, "%d", n)
}

// Quiet is a concrete writer whose Write is pure.
type Quiet struct{}

func (Quiet) Write(p []byte) (int, error) { return len(p), nil } // want Write:`map\[\]`

// ToQuiet passes a concrete type whose Write is pure: nothing.
//
//effectgo:pure
func ToQuiet(n int) { // want ToQuiet:`map\[\]`
	fmt.Fprintf(Quiet{}, "%d", n)
}
