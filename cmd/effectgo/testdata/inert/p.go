package p

import (
	"errors"
	"fmt"
	"net/http"
)

var ErrX = errors.New("x")

// C1: errors contract on a func with no error result.
//
//effectgo:errors ErrX
func NoErrResult() int { return 1 }

// C2: handler on net/http func.
//
//effectgo:handler mapErr
func H(w http.ResponseWriter, r *http.Request) {}

//effectgo:errmap
func mapErr(err error) error { return err }

// C3: sealed on a struct.
//
//effectgo:sealed
type S struct{}

// C4: sealed on an int with no constants.
//
//effectgo:sealed
type Lonely int

// C5: mustuse on a func with no results.
//
//effectgo:mustuse
func NoResults() {}

// C6: pure on a var.
//
//effectgo:pure
var V = func() { fmt.Println("x") }

// C7: package directive on a function.
//
//effectgo:package pure
func PkgOnFunc() { fmt.Println("x") }

// C8: directive inside a body.
func InBody() {
	//effectgo:pure
	fmt.Println("x")
}

// C9: spaced directive.
//
// effectgo:pure
func Spaced() { fmt.Println("x") }

// C10: detached by blank line.

//effectgo:pure

func Detached() { fmt.Println("x") }

// C11: errmap with wrong shape.
//
//effectgo:errmap
func BadMap(a, b int) string { return "" }

// C12: allow with no effects.
//
//effectgo:allow
func AllowNothing() { fmt.Println("x") }

// C13: pure on a func literal assigned in body.
func Lit() {
	f := //effectgo:pure
		func() { fmt.Println("x") }
	f()
}
