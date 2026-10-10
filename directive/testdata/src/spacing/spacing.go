// Package spacing has comments with a space after the slashes.
package spacing

// Spaced has a directive with a space after the slashes.
//
// effectgo:pure // want `// effectgo:pure is not a directive: remove the space after //`
func Spaced() {}

// Prose mentions effectgo:pure, which is not a directive at the start of a comment.
func Prose() {}

// effectgo:frobnicate is prose: frobnicate is not a directive keyword.
func Unknown() {}

// // effectgo:handler is a comment inside a comment, not a directive.
func Nested() {}

// effectgo:allow db // want `// effectgo:allow is not a directive: remove the space after //`
var Vv = 1

// effectgo:sealed // want `// effectgo:sealed is not a directive: remove the space after //`
type S int

func Body() {
	// effectgo:mustuse // want `// effectgo:mustuse is not a directive: remove the space after //`
	_ = 1
}
