package c

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// Without stub files the writers are not silent: errset cannot prove their
// error sets empty (see package w for the stubbed case).
func Use(f *os.File) {
	fmt.Println("x") // fmt.Print* is allowed
	fmt.Fprintf(os.Stdout, "x")
	var buf bytes.Buffer
	buf.WriteString("x") // want "result of \\(\\*bytes.Buffer\\).WriteString is discarded"
	var sb strings.Builder
	sb.WriteByte('x') // want "result of \\(\\*strings.Builder\\).WriteByte is discarded"
	f.Close()         // want "result of \\(\\*os.File\\).Close is discarded"
}
