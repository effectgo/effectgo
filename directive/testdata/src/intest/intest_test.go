package intest

import "testing"

// TestEnum is an enum declared in a test file. exhaustive does not read the
// directive there.
//
//effectgo:sealed // want `//effectgo:sealed is not read in a _test.go file: sealed types are checked only in non-test files`
type TestEnum int

// TestPure is a test function with a contract: effects reads it in test files too.
//
//effectgo:pure
func TestPure(t *testing.T) {}
