package w

import (
	"bytes"
	"hash"
	"strings"
)

// With the stdlib stub file loaded, the writers that never fail are silent.
func Writers(buf *bytes.Buffer, sb *strings.Builder, h hash.Hash) {
	buf.Write(nil)
	buf.WriteString("x")
	buf.WriteByte('x')
	buf.WriteRune('x')
	sb.Write(nil)
	sb.WriteString("x")
	sb.WriteByte('x')
	sb.WriteRune('x')
	h.Write(nil)
}
