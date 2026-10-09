package c

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

func Use(f *os.File) {
	fmt.Println("x") // fmt.Print* is allowed
	fmt.Fprintf(os.Stdout, "x")
	var buf bytes.Buffer
	buf.WriteString("x") // allowed by the built-in list
	var sb strings.Builder
	sb.WriteByte('x')
	f.Close() // want "result of \\(\\*os.File\\).Close is discarded"
}
