// Package shipped checks the stub file that ships with the analyzer
// (stubs/stdlib.stubs).
package shipped

import (
	"fmt"
	"io"
)

// ErrStr is the documented error.Error false positive. Pure with the shipped stubs.
//
//effectgo:pure
func ErrStr(err error) string { return err.Error() } // want ErrStr:`map\[\]`

// Show calls fmt.Stringer.String, which the shipped stubs declare pure.
//
//effectgo:pure
func Show(s fmt.Stringer) string { return s.String() } // want Show:`map\[\]`

// Copy calls io.Copy, which the shipped stubs declare unknown.
//
//effectgo:pure
func Copy(w io.Writer, r io.Reader) error { // want Copy:`map\[unknown:` `Copy is declared //effectgo:pure but has effect unknown via Copy → io\.Copy`
	_, err := io.Copy(w, r)
	return err
}

// Wrap uses an io constructor, which the shipped stubs declare pure.
//
//effectgo:pure
func Wrap(r io.Reader) io.Reader { return io.LimitReader(r, 10) } // want Wrap:`map\[\]`
