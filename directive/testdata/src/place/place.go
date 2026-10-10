// Package place holds the directive placements that the analyzers read, and the
// placements that none of them reads.
//
//effectgo:package pure
package place

import "errors"

var ErrX = errors.New("x")

// Pure is a function with a contract: read by effects, so silent.
//
//effectgo:pure
func Pure() {}

// Errs has an errors contract and an error result: read by errset, so silent.
//
//effectgo:errors ErrX
func Errs() error { return ErrX }

// Handler and mapErr are read by errset, so silent.
//
//effectgo:handler mapErr
func Handler() func() error { return func() error { return nil } }

//effectgo:errmap
func mapErr(err error) error { return err }

// MustFn has the mustuse directive on a function: the mustuse analyzer reports it,
// so this analyzer is silent.
//
//effectgo:mustuse
func MustFn() int { return 1 }

// PkgOnFunc has the package directive on a function: the effects analyzer reports it.
//
//effectgo:package pure
func PkgOnFunc() {}

// Repo is an interface with a contract on the type: read by effects, so silent.
//
//effectgo:allow db
type Repo interface {
	// Get is a method with its own contract: silent.
	//
	//effectgo:pure
	Get(id int) int

	// Find is a method with an errors contract: silent.
	//
	//effectgo:errors ErrX
	Find(id int) error
}

// Receipt is a must-use type; the directive is on its type declaration: silent.
//
//effectgo:mustuse
type Receipt struct{ ID string }

// Status is an enum; the directive is on the named integer type: silent.
//
//effectgo:sealed
type Status int

// Shape is sealed by its own unexported method, and the directive states it: silent.
//
//effectgo:sealed
type Shape interface{ isShape() }

// Grouped is a one-type group whose doc comment is the directive's: silent.
//
//effectgo:mustuse
type (
	Grouped struct{}
)

// V is a var with a directive that no analyzer reads.
//
//effectgo:pure // want `//effectgo:pure is not attached to a function, interface or type declaration and is not read`
var V = 1

// C is a const with a trailing directive that no analyzer reads.
const C = 2 //effectgo:mustuse // want `//effectgo:mustuse is not attached to a function, interface or type declaration and is not read`

// P is a var with the package directive: effects reads it only in the package doc.
//
//effectgo:package pure // want `//effectgo:package is only valid in a package doc comment`
var P = 3

// LitVar assigns a function literal to a var; the directive on the literal is not read.
var LitVar = //effectgo:pure // want `//effectgo:pure is not attached to a function, interface or type declaration and is not read`
func() {}

// InBody has directives inside its body, where no analyzer reads them.
func InBody() int {
	//effectgo:pure // want `//effectgo:pure is not attached to a function, interface or type declaration and is not read`
	x := 1
	f := //effectgo:pure // want `//effectgo:pure is not attached to a function, interface or type declaration and is not read`
		func() int { return x }
	return f()
}

// LocalIface declares an interface in a body: no analyzer reads its contract.
func LocalIface() {
	// Local is the interface of this body.
	//
	//effectgo:allow db // want `//effectgo:allow is not attached to a function, interface or type declaration and is not read`
	type Local interface{ M() }
}

// Holder has a field with a directive that no analyzer reads.
type Holder struct {
	// Field is a struct field.
	//
	//effectgo:pure // want `//effectgo:pure is not attached to a function, interface or type declaration and is not read`
	Field int
}

// Embedder embeds an interface; a directive on the embedded interface is not read.
type Embedder interface {
	//effectgo:allow db // want `//effectgo:allow is not attached to a function, interface or type declaration and is not read`
	Repo
}

// Detached has a directive that a blank line separates from the function.

//effectgo:pure // want `//effectgo:pure is not attached to a function, interface or type declaration and is not read`

func Detached() {}

func Trailing() {} //effectgo:pure // want `//effectgo:pure is not attached to a function, interface or type declaration and is not read`

// Plain is a struct; pure applies to functions and interfaces.
//
//effectgo:pure // want `//effectgo:pure on type Plain is not read: pure applies to functions, interfaces and interface methods`
type Plain struct{}

// Named is a defined type; errors applies to functions and interfaces.
//
//effectgo:errors ErrX // want `//effectgo:errors on type Named is not read: errors applies to functions, interfaces and interface methods`
type Named int

// HandlerType is a named type; handler applies to functions.
//
//effectgo:handler mapErr // want `//effectgo:handler on type HandlerType is not read: handler applies to functions`
type HandlerType string

// SealedFunc is a function; sealed applies to types.
//
//effectgo:sealed // want `//effectgo:sealed on function SealedFunc is not read: sealed applies to interfaces and named integer or string types`
func SealedFunc() {}

// Sealer has a directive on a method; sealed applies to types.
type Sealer interface {
	isSealer()

	// Mark is a method.
	//
	//effectgo:sealed // want `//effectgo:sealed on method Sealer.Mark is not read: sealed applies to interfaces and named integer or string types`
	Mark()
}

// Repo2 has a directive on a method; mustuse applies to named types.
type Repo2 interface {
	// Do is a method.
	//
	//effectgo:mustuse // want `//effectgo:mustuse on method Repo2.Do is not read: mustuse applies to named type declarations`
	Do()
}

// Shadow's group has a directive; its type has a doc comment of its own, which is
// the one the interface contract reads.
//
//effectgo:pure // want `//effectgo:pure is in the doc comment of a type group, and the type has a doc comment of its own`
type (
	// Shadow is the type.
	Shadow interface{ M() }
)

// Several declares two types in one group with a directive on the group.
//
//effectgo:mustuse // want `//effectgo:mustuse is in the doc comment of a group of several types and is not read: put it on each type`
type (
	A struct{}
	B struct{}
)

// NativeFn has no body, so errset does not check its contract.
//
//effectgo:errors ErrX // want `//effectgo:errors on function NativeFn is not read: NativeFn has no body`
func NativeFn() error

// Unknown has a keyword that no analyzer knows, on a var.
//
//effectgo:purr // want `//effectgo:purr is not attached to a function, interface or type declaration and is not read`
var Unknown = 4
