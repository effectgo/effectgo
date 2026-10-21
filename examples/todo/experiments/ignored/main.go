// Command ignored shows what the Go type system does and does not catch when
// a Result is discarded or Unwrap is called on an Err. It compiles and vets
// cleanly; the last line panics at runtime on purpose.
package main

import (
	"fmt"

	"github.com/effectgo/effectgo/examples/todo/domain"
)

func main() {
	svc := domain.NewService()

	// 1. A Result discarded as a bare statement: compiles, vets, no warning.
	svc.GetR(1)

	// 2. Unwrap on an Err: compiles, vets, panics at runtime.
	r := svc.GetR(1)
	fmt.Println("IsOk:", r.IsOk())
	fmt.Println("unwrapping an Err now...")
	fmt.Println(r.Unwrap())
}
