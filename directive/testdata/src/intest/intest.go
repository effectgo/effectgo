// Package intest checks the sealed directive in a test file.
package intest

// Enum is an enum declared in a non-test file: exhaustive reads its directive.
//
//effectgo:sealed
type Enum int
