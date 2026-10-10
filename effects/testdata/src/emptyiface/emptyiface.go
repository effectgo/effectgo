package emptyiface

// Reader is a plain interface with a method and no contract.
type Reader interface{ Read() int }

// Base only embeds Reader. Its type-level contract covers the methods it declares,
// and it declares none, so the contract checks nothing.
//
//effectgo:pure
type Base interface{ Reader } // want `effectgo:pure on Base checks nothing: an interface contract covers the methods it declares, and Base declares none`

// Allowed is the same with an allow list.
//
//effectgo:allow db
type Allowed interface{ Reader } // want `effectgo:allow on Allowed checks nothing: an interface contract covers the methods it declares, and Allowed declares none`

// Has declares a method of its own: the type-level contract covers it, and nothing is
// reported.
//
//effectgo:pure
type Has interface {
	Reader
	Get() int // want Get:"."
}
