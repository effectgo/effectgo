// Package diag is the diagnostics port of the example.
package diag

// Writer is the diagnostics port. Its implementations write to standard I/O.
//
//effectgo:allow stdio
type Writer interface {
	Write(p []byte) (int, error)
}
