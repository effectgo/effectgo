package shape

// Reader is a plain interface.
type Reader interface{ Read() int }

// Embedded only embeds Reader. Its errors contract covers the methods it declares, and
// it declares none, so the contract checks nothing.
//
//effectgo:errors ErrX
type Embedded interface{ Reader } // want `effectgo:errors on Embedded checks nothing: an interface contract covers the methods it declares, and Embedded declares none`

// Has declares a method of its own: the contract covers it, and nothing is reported.
//
//effectgo:errors ErrX
type Has interface {
	Reader
	Get() error // want Get:"."
}
