// Package enumdef stands in for rt/schema: an Enum interface whose Valid method
// is pure by contract.
package enumdef

// Enum is the contract of an enum type that a decoder asks to validate.
type Enum interface {
	//effectgo:pure
	Valid() bool // want Valid:`&\{\[\]\}`
}

// Check calls Valid through the contract.
//
//effectgo:pure
func Check(v Enum) bool { return v.Valid() } // want Check:`map\[\]`
