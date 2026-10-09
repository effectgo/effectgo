// Package via exercises via items of stub rules: a stdlib function that calls a
// method of an argument performs that method's effects, evaluated at the call
// site from the argument's static type (effects/via.go). The shipped stdlib
// stubs are used (effects_test.go, TestViaItems).
package via

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// NetWriter is an interface with a contract: its writes are net.
//
//effectgo:allow net
type NetWriter interface {
	Write(p []byte) (int, error) // want Write:`\[net\]`
}

// ToWriter writes through an io.Writer parameter, which has no contract, so the
// effect is unknown, as it is for a direct call of w.Write.
//
//effectgo:pure
func ToWriter(w io.Writer, n int) { // want ToWriter:`unknown` `ToWriter is declared //effectgo:pure but has effect unknown via ToWriter → fmt\.Fprintf → io\.Writer\.Write`
	fmt.Fprintf(w, "%d\n", n)
}

// Direct calls the method itself: the same effect as ToWriter.
//
//effectgo:pure
func Direct(w io.Writer) { // want Direct:`unknown` `Direct is declared //effectgo:pure but has effect unknown via Direct → io\.Writer\.Write`
	_, _ = w.Write([]byte("x"))
}

// ToStdout writes to os.Stdout: stdio.
//
//effectgo:pure
func ToStdout(n int) { // want ToStdout:`stdio` `ToStdout is declared //effectgo:pure but has effect stdio`
	fmt.Fprintf(os.Stdout, "%d\n", n)
}

// ToBuffer and ToBuilder write to values whose Write is pure: nothing.
//
//effectgo:pure
func ToBuffer(n int) string { // want ToBuffer:`map\[\]`
	var b bytes.Buffer
	fmt.Fprintf(&b, "%d", n)
	return b.String()
}

//effectgo:pure
func ToBuilder(n int) string { // want ToBuilder:`map\[\]`
	var b strings.Builder
	fmt.Fprintln(&b, n)
	return b.String()
}

// ToContract writes through an interface with a contract: the contract's effect.
//
//effectgo:pure
func ToContract(w NetWriter) { // want ToContract:`net` `ToContract is declared //effectgo:pure but has effect net via ToContract → fmt\.Fprintf → \(NetWriter\)\.Write \[contract\]`
	fmt.Fprintf(w, "x")
}

// CopyBoth reads a file and writes a buffer: the file's Read is fs, and the
// buffer's ReadFrom and Write are pure.
//
//effectgo:pure
func CopyBoth(b *bytes.Buffer, f *os.File) { // want CopyBoth:`fs` `CopyBoth is declared //effectgo:pure but has effect fs`
	_, _ = io.Copy(b, f)
}

// CopyIface copies between two interfaces: unknown.
//
//effectgo:pure
func CopyIface(dst io.Writer, src io.Reader) { // want CopyIface:`unknown` `CopyIface is declared //effectgo:pure but has effect unknown`
	_, _ = io.Copy(dst, src)
}

// ReadInt reads through an io.Reader parameter.
//
//effectgo:pure
func ReadInt(r io.Reader) (int, error) { // want ReadInt:`unknown` `ReadInt is declared //effectgo:pure but has effect unknown via ReadInt → fmt\.Fscan → io\.Reader\.Read`
	var n int
	_, err := fmt.Fscan(r, &n)
	return n, err
}

// WriteStr uses io.WriteString on an io.Writer: unknown.
//
//effectgo:pure
func WriteStr(w io.Writer) { // want WriteStr:`unknown` `WriteStr is declared //effectgo:pure but has effect unknown`
	_, _ = io.WriteString(w, "x")
}

// WriteStrBuf uses io.WriteString on a *bytes.Buffer: pure.
//
//effectgo:pure
func WriteStrBuf() string { // want WriteStrBuf:`map\[\]`
	var b bytes.Buffer
	_, _ = io.WriteString(&b, "x")
	return b.String()
}

// Discard writes to io.Discard, whose writes do nothing: pure.
//
//effectgo:pure
func Discard(n int) { // want Discard:`map\[\]`
	fmt.Fprintf(io.Discard, "%d", n)
}

// Encode writes through a stored writer: the stored value is not tracked.
//
//effectgo:pure
func Encode(w io.Writer, v any) error { // want Encode:`unknown` `Encode is declared //effectgo:pure but has effect unknown`
	return json.NewEncoder(w).Encode(v)
}

// Buffered writes through a *bufio.Writer: its methods are unknown.
//
//effectgo:pure
func Buffered(bw *bufio.Writer) { // want Buffered:`unknown` `Buffered is declared //effectgo:pure but has effect unknown`
	bw.WriteString("x")
}

// NewBuffered only builds a bufio.Writer: pure.
//
//effectgo:pure
func NewBuffered(b *bytes.Buffer) *bufio.Writer { // want NewBuffered:`map\[\]`
	return bufio.NewWriter(b)
}
