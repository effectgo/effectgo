// Package enumplain implements enumdef.Enum without importing it. The conformance
// check reads the contracts of the packages a package imports, so it does not see
// this implementation: its Valid reads the clock and is not reported. This test
// locks that gap; see docs/reference/analyzers.md (schematag).
package enumplain

import "time"

// Kind satisfies enumdef.Enum structurally.
type Kind string

func (k Kind) Valid() bool { return time.Now().Unix() > 0 && k != "" } // want Valid:`map\[time:`
