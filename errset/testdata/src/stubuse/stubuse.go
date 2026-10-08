// Package stubuse calls stubbed functions (report flag and stubs flag on).
package stubuse

import (
	"strconv"

	"stubdep"
)

// The stub gives a sentinel and an abstract interface variant.
func useLookup(id int) error { // want useLookup:"." "errors of useLookup: stubdep.ErrA | some stubdep.Status"
	return stubdep.Lookup(id)
}

// The stub says none, so the opaque error of Hard is gone.
func useHard() error { // want useHard:"." "errors of useHard: \\(none\\)"
	return stubdep.Hard()
}

// The stub on the interface method beats its contract.
func useFind(r stubdep.Repo) error { // want useFind:"." "errors of useFind: some stubdep.Status"
	return r.Find(1)
}

// The stub on a standard library function.
func useAtoi(s string) error { // want useAtoi:"." "errors of useAtoi: strconv.NumError"
	_, err := strconv.Atoi(s)
	return err
}
