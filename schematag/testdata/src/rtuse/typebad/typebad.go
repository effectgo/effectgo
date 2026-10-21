// Package typebad has field types that rt/schema does not decode, and field
// declarations that Compile rejects.
package typebad

import (
	"fmt"

	"github.com/effectgo/effectgo/rt/schema"
)

// Inner is embedded below.
type Inner struct {
	X int `json:"x"`
}

// Bad covers the unsupported types.
type Bad struct {
	Ch       chan int         `json:"ch"`    // want `type chan int is not supported`
	Fn       func()           `json:"fn"`    // want `type func\(\) is not supported`
	Arr      [2]int           `json:"arr"`   // want `type \[2\]int is not supported`
	Bytes    []byte           `json:"bytes"` // want `byte slices are not supported`
	Keyed    map[int]string   `json:"keyed"` // want `map key must be a string kind, got int`
	Iface    fmt.Stringer     `json:"iface"` // want `non-empty interface fmt.Stringer is not supported`
	Inner                     // want `embedded fields are not supported`
	A        string           `json:"x"`
	B        string           `json:"x"`       // want `json name "x" is used by two fields of Bad`
	hidden   string           `schema:"trim"`  // want `unexported field has a schema tag`
	Complex  complex128       `json:"complex"` // want `type complex128 is not supported`
	Uptr     uintptr          `json:"uptr"`    // want `type uintptr is not supported`
	Nested   *[]chan int      `json:"nested"`  // want `type chan int is not supported`
	Anything any              `json:"anything"`
	Keys     map[string]Inner `json:"keys"`
	Ok       map[Key]Inner    `json:"ok"`
}

// Key is a string kind: a valid map key.
type Key string

// Decode reads Bad.
func Decode(data []byte) (Bad, error) {
	return schema.Decode[Bad](data)
}

// Compile checks Bad without decoding.
func Compile() error {
	return schema.Compile[Bad]()
}
