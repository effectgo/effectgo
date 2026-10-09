package b

import "a"

func Use() {
	a.Pay()  // want "result of a.Pay is discarded"
	a.Mark() // want "result of a.Mark is discarded"
	_ = a.Mark()
}
