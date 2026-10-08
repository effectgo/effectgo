//go:build ignore

// This file is excluded from ./... builds (go:build ignore). It is compiled
// on its own to show the compiler rejecting an unused error variable.
package main

import (
	"fmt"

	"github.com/faintghost/effectgo/examples/todo/domain"
)

func main() {
	svc := domain.NewService()
	v, err := svc.Get(1) // err is never used
	fmt.Println(v)
}
