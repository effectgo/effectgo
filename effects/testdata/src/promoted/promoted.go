// Package promoted checks the methods that a type gets from an embedded interface
// or struct, against the contracted interface Writer. A method promoted from an
// embedded interface is checked with the effects of a call through that interface.
package promoted

import (
	"io"
	"net/http"
	"time"

	"promotedlib"
)

// Writer is the diagnostics port: its implementations write to standard I/O.
//
//effectgo:allow stdio
type Writer interface {
	Write(p []byte) (int, error) // want Write:`&\{\[stdio\]\}`
}

// statusWriter promotes Write from http.ResponseWriter, which writes to the client.
type statusWriter struct {
	http.ResponseWriter // want `\(\*statusWriter\)\.Write violates contract of \(Writer\)\.Write \[allow stdio\]: has effect net via \(\*statusWriter\)\.Write \[promoted from embedded http\.ResponseWriter\] → http\.\(ResponseWriter\)\.Write \[stub\]`
	status              int
}

// NewStatusWriter returns the response as a Writer.
func NewStatusWriter(w http.ResponseWriter) Writer { return &statusWriter{ResponseWriter: w} } // want NewStatusWriter:`map\[\]`

// ownWriter promotes its Header from http.ResponseWriter and defines its own Write.
// The type's own method is the one that is checked, and it has no effects.
type ownWriter struct {
	http.ResponseWriter
}

// Write is the type's own method: it writes nothing.
func (ownWriter) Write(p []byte) (int, error) { return len(p), nil } // want Write:`map\[\]`

// NewOwnWriter returns ownWriter as a Writer.
func NewOwnWriter(w http.ResponseWriter) Writer { return ownWriter{ResponseWriter: w} } // want NewOwnWriter:`map\[\]`

// unknownWriter promotes Write from io.Writer, whose effects no stub gives: unknown.
type unknownWriter struct {
	io.Writer // want `\(\*unknownWriter\)\.Write violates contract of \(Writer\)\.Write \[allow stdio\]: has effect unknown via \(\*unknownWriter\)\.Write \[promoted from embedded io\.Writer\] → io\.Writer\.Write`
}

// NewUnknownWriter returns unknownWriter as a Writer.
func NewUnknownWriter(w io.Writer) Writer { return &unknownWriter{Writer: w} } // want NewUnknownWriter:`map\[\]`

// stdioWriter promotes Write from the contracted Writer it embeds: the effects are
// those of Writer's contract, which Writer allows.
type stdioWriter struct {
	Writer
}

// NewStdioWriter returns stdioWriter as a Writer.
func NewStdioWriter(w Writer) Writer { return stdioWriter{Writer: w} } // want NewStdioWriter:`map\[\]`

// innerClock writes to the clock, so its own Write violates Writer.
type innerClock struct{}

func (*innerClock) Write(p []byte) (int, error) { // want `\(\*innerClock\)\.Write violates contract of \(Writer\)\.Write \[allow stdio\]: has effect time via \(\*innerClock\)\.Write → time\.Now` Write:`map\[time:`
	_ = time.Now()
	return len(p), nil
}

// outerClock promotes Write from *innerClock, a struct of this package. The violation
// is that of innerClock.Write, reported once at its method.
type outerClock struct {
	*innerClock
}

// NewOuterClock returns outerClock as a Writer.
func NewOuterClock() Writer { return &outerClock{innerClock: &innerClock{}} } // want NewOuterClock:`map\[\]`

// foreignOuter promotes Write from *promotedlib.Sink, a struct of another package
// whose Write reads the clock. The violation is reported at the type's name.
type foreignOuter struct { // want `\(\*Sink\)\.Write violates contract of \(Writer\)\.Write \[allow stdio\]: has effect time via promotedlib\.\(\*Sink\)\.Write → time\.Now`
	*promotedlib.Sink
}

// NewForeignOuter returns foreignOuter as a Writer.
func NewForeignOuter() Writer { return &foreignOuter{Sink: &promotedlib.Sink{}} } // want NewForeignOuter:`map\[\]`
