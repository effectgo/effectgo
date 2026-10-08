// Package stubbed exercises stub overrides: a stub replaces the taxonomy, and
// supplies effects for an interface call that is otherwise unknown.
package stubbed

import (
	"io"
	"time"
)

// Now is a time effect under the taxonomy. The test stub says pure.
//
//effectgo:pure
func Now() time.Time { return time.Now() } // want Now:`map\[\]`

// Send calls an interface method. The test stub supplies net, which allow net admits.
//
//effectgo:allow net
func Send(w io.Writer) error { // want Send:`map\[net:`
	_, err := w.Write(nil)
	return err
}

// SendPure claims purity; the stub-supplied net effect is a violation.
//
//effectgo:pure
func SendPure(w io.Writer) error { // want SendPure:`map\[net:` `SendPure is declared //effectgo:pure but has effect net via SendPure → io\.\(Writer\)\.Write \[stub\]`
	_, err := w.Write(nil)
	return err
}

// Fail is named by an errors line in the test stub file. That line is ignored
// by the effects analyzer.
func Fail() {} // want Fail:`map\[\]`
