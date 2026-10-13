package astypehandler

import "astypemap"

// LostError is not mapped by astypemap.Mapper.
type LostError struct{}

func (*LostError) Error() string { return "lost" } // want Error:"."

// handle returns a mapped error and an unmapped one.
//
//effectgo:handler astypemap.Mapper
func handle(n int) error { // want "handler handle may return astypehandler.LostError" handle:"."
	if n == 0 {
		return &astypemap.BusyError{}
	}
	return &LostError{}
}

// handleBusy returns only the mapped error.
//
//effectgo:handler astypemap.Mapper
func handleBusy() error { // want handleBusy:"."
	return &astypemap.BusyError{}
}
