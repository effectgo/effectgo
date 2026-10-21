// Package schema is the testdata stand-in for rt/schema. Its signatures match
// the real package; the analyzer matches the import path and the names only.
package schema

import "regexp"

// Enum is the pure contract of an enum type.
type Enum interface {
	//effectgo:pure
	Valid() bool
}

// Decode decodes a T.
func Decode[T any](data []byte) (T, error) {
	var z T
	return z, nil
}

// Compile checks that T can be decoded.
func Compile[T any]() error { return nil }

// RegisterPattern makes a regular expression available to pattern=NAME.
func RegisterPattern(name string, re *regexp.Regexp) {}
