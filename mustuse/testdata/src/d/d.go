package d

import "os"

// Box is a generic must-use type. Every instance is must-use.
//
//effectgo:mustuse
type Box[T any] struct{ V T } // want Box:"mustuse\\(Box\\)"

// Receipt is an alias of a marked instance: the mark is seen through it.
type Receipt = Box[int]

// Aliased carries the directive on an alias, which is reported.
//
//effectgo:mustuse
type Aliased = Box[string] // want "applies to a defined type, not an alias"

func MakeBox[T any]() Box[T] { return Box[T]{} }

func MakeAlias() Receipt { return Receipt{} }

// Gen returns an error through a generic function.
func Gen[T any]() error { return nil } // want Gen:"errorfree\\(Gen\\)"

func Use(f *os.File, g func() error) {
	MakeBox[int]() // want "result of d.MakeBox is discarded"
	MakeAlias()    // want "result of d.MakeAlias is discarded"
	Gen[string]()  // provably no error
	closer := f.Close
	closer() // want "result of \\(\\*os.File\\).Close is discarded"
	var h func() error = f.Close
	h() // want "result of \\(\\*os.File\\).Close is discarded"
	m := f.Close
	m = g
	m() // want "result of m is discarded"
	n := f.Close
	_ = &n
	n() // want "result of n is discarded"
}
