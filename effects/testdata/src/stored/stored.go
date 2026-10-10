// Package stored exercises stored-value items: a constructor that keeps its
// writer or reader (stores:0), and the methods that act through the stored value
// (via:stored.Write, via:stored.Read). A receiver whose stored value is not
// known is unknown.
package stored

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
)

// ChainBuf encodes to a *bytes.Buffer through a chain: pure.
//
//effectgo:pure
func ChainBuf(v any) error { // want ChainBuf:`map\[\]`
	var b bytes.Buffer
	return json.NewEncoder(&b).Encode(v)
}

// ChainIface encodes through an io.Writer parameter: unknown, as w.Write is.
//
//effectgo:pure
func ChainIface(w io.Writer, v any) error { // want ChainIface:`unknown` `ChainIface is declared //effectgo:pure but has effect unknown via ChainIface → json\.\(\*Encoder\)\.Encode → io\.Writer\.Write`
	return json.NewEncoder(w).Encode(v)
}

// LocalFile encodes through a local encoder over an *os.File: fs.
//
//effectgo:pure
func LocalFile(f *os.File, v any) error { // want LocalFile:`fs` `LocalFile is declared //effectgo:pure but has effect fs`
	enc := json.NewEncoder(f)
	return enc.Encode(v)
}

// LocalStdout encodes to os.Stdout through a local encoder: stdio.
//
//effectgo:pure
func LocalStdout(v any) error { // want LocalStdout:`stdio` `LocalStdout is declared //effectgo:pure but has effect stdio`
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(v)
}

// Holder keeps an encoder in a field: the stored value is not known.
type Holder struct {
	enc *json.Encoder
}

// Put encodes through a field: unknown.
//
//effectgo:pure
func (h *Holder) Put(v any) error { // want Put:`unknown` `Put is declared //effectgo:pure but has effect unknown`
	return h.enc.Encode(v)
}

// PutParam encodes through a parameter of type *json.Encoder: unknown.
//
//effectgo:pure
func PutParam(enc *json.Encoder, v any) error { // want PutParam:`unknown` `PutParam is declared //effectgo:pure but has effect unknown`
	return enc.Encode(v)
}

func keep(*json.Encoder) {} // want keep:`map\[\]`

// Escaped passes the encoder to another function, which could keep it: unknown.
//
//effectgo:pure
func Escaped(b *bytes.Buffer, v any) error { // want Escaped:`unknown` `Escaped is declared //effectgo:pure but has effect unknown`
	enc := json.NewEncoder(b)
	keep(enc)
	return enc.Encode(v)
}

// Reassigned has two assignments to the encoder: unknown.
//
//effectgo:pure
func Reassigned(b *bytes.Buffer, f *os.File, v any) error { // want Reassigned:`unknown` `Reassigned is declared //effectgo:pure but has effect unknown`
	enc := json.NewEncoder(b)
	enc = json.NewEncoder(f)
	return enc.Encode(v)
}

// ResetCase re-points a bufio reader with Reset: the reader is not trusted.
//
//effectgo:pure
func ResetCase(b *bytes.Buffer, f *os.File) (string, error) { // want ResetCase:`unknown` `ResetCase is declared //effectgo:pure but has effect unknown`
	br := bufio.NewReader(b)
	br.Reset(f)
	return br.ReadString('\n')
}

// BufPure writes through a bufio writer over a *bytes.Buffer: pure.
//
//effectgo:pure
func BufPure() string { // want BufPure:`map\[\]`
	var b bytes.Buffer
	bw := bufio.NewWriter(&b)
	_, _ = bw.WriteString("x")
	_ = bw.Flush()
	return b.String()
}

// ReadRows reads a CSV file through a local reader over an *os.File, setting a
// field on the reader first: fs (the field write does not change the reader).
//
//effectgo:pure
func ReadRows(f *os.File) ([]string, error) { // want ReadRows:`fs` `ReadRows is declared //effectgo:pure but has effect fs`
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rec, err := r.Read()
	return rec, err
}

// CopyReader reads through an io.Reader parameter: unknown.
//
//effectgo:pure
func CopyReader(r io.Reader) (int, error) { // want CopyReader:`unknown` `CopyReader is declared //effectgo:pure but has effect unknown`
	br := bufio.NewReader(r)
	s, err := br.ReadString('\n')
	return len(s), err
}
