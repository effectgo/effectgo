package a

import "os"

type Receipt struct{ ID int }

//effectgo:mustuse
type Charge struct{ Amount int } // want Charge:"mustuse\\(Charge\\)"

type Result struct{ OK bool }

func Pay() error { return nil }

func Quote() (int, error) { return 1, nil }

func Mark() *Charge { return &Charge{} }

func Plain() int { return 1 }

func Use() {
	Pay()     // want "result of a.Pay is discarded; handle it or assign to _"
	Quote()   // want "result of a.Quote is discarded"
	Mark()    // want "result of a.Mark is discarded"
	Plain()   // no result type is must-use
	_ = Pay() // explicit discard
	_, _ = Quote()
	go Pay()       // go statements are not reported
	defer Pay()    // defer statements are not reported
	os.Remove("x") // want "result of os.Remove is discarded"
	var r Result
	_ = r
	f := Pay
	f() // want "result of f is discarded"
}
