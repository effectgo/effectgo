package wn

import (
	"bytes"
	"hash"
	"strings"
)

// Without stub files the same calls are reported: errset cannot
// prove their error sets empty, and the former built-in allowlist is gone.
func Writers(buf *bytes.Buffer, sb *strings.Builder, h hash.Hash, h32 hash.Hash32) {
	buf.Write(nil)      // want "result of \\(\\*bytes.Buffer\\).Write is discarded"
	sb.WriteString("x") // want "result of \\(\\*strings.Builder\\).WriteString is discarded"
	h.Write(nil)        // want "result of \\(io.Writer\\).Write is discarded"
	h32.Write(nil)      // want "result of \\(io.Writer\\).Write is discarded"
}
