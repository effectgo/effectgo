package fn

// NoResults has the directive and no results: it checks nothing.
//
//effectgo:mustuse
func NoResults() {} // want `effectgo:mustuse on NoResults checks nothing: it has no results`

// Int has the directive on a function whose result is a basic type.
//
//effectgo:mustuse
func Int() int { return 1 } // want `effectgo:mustuse on Int checks nothing: //effectgo:mustuse marks a named type, not a function`

type T struct{}

// Method has the directive on a method.
//
//effectgo:mustuse
func (T) Method() int { return 0 } // want `effectgo:mustuse on T.Method checks nothing`

// Receipt is marked on its own type declaration, which is the right place.
//
//effectgo:mustuse
type Receipt struct{ ID string } // want Receipt:"."

// Make returns a marked type. The directive is on Receipt, so Make is silent.
func Make() Receipt { return Receipt{} }
