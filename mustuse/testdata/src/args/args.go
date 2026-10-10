package args

// Marked has a trailing comment after the directive. The comment is not an argument,
// and the type is must-use.
//
//effectgo:mustuse // the caller must handle it
type Marked struct{} // want Marked:"."

// Extra has an argument after the directive. The argument is reported, and the type is
// still must-use.
//
//effectgo:mustuse extra // want `//effectgo:mustuse takes no arguments`
type Extra struct{} // want Extra:"."

// Make returns a Marked value.
func Make() Marked { return Marked{} }

// MakeExtra returns an Extra value.
func MakeExtra() Extra { return Extra{} }

func use() {
	Make()      // want `result of args.Make is discarded: args.Marked is //effectgo:mustuse`
	MakeExtra() // want `result of args.MakeExtra is discarded: args.Extra is //effectgo:mustuse`
}
