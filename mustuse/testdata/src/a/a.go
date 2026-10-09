package a

import (
	"errors"
	"os"
)

// ErrX is a sentinel: a function that returns it has a non-empty error set.
var ErrX = errors.New("x")

type Receipt struct{ ID int }

//effectgo:mustuse
type Charge struct{ Amount int } // want Charge:"mustuse\\(Charge\\)"

type Result struct{ OK bool }

func Pay() error { return ErrX }

func Quote() (int, error) { return 1, ErrX }

// Nil always returns nil, so its error set is empty and a discard is not
// reported.
func Nil() error { return nil } // want Nil:"errorfree\\(Nil\\)"

func Quiet() (int, error) { return 1, nil } // want Quiet:"errorfree\\(Quiet\\)"

func Mark() *Charge { return &Charge{} }

// Both returns a must-use value and an error that is always nil. The error
// is not reported, but the must-use value is.
func Both() (*Charge, error) { return &Charge{}, nil } // want Both:"errorfree\\(Both\\)"

func Plain() int { return 1 }

func Use() {
	Pay()     // want "result of a.Pay is discarded; handle it or assign to _"
	Quote()   // want "result of a.Quote is discarded"
	Mark()    // want "result of a.Mark is discarded: \\*a.Charge is //effectgo:mustuse"
	Plain()   // no result type is must-use
	Nil()     // provably no error
	Quiet()   // provably no error
	_ = Pay() // explicit discard
	_, _ = Quote()
	go Pay()       // go statements are not reported
	defer Pay()    // defer statements are not reported
	os.Remove("x") // want "result of os.Remove is discarded"
	var r Result
	_ = r
	f := Pay
	f() // want "result of a.Pay is discarded"
}
