package forms

import "errors"

var ErrX = errors.New("x")

// Noted has a trailing comment after its mustuse directive. The comment is not an
// argument, so the type is must-use.
//
//effectgo:mustuse // the caller must handle it
type Noted struct{}

// MakeNoted returns a Noted.
func MakeNoted() Noted { return Noted{} }

// Extra has an argument after its mustuse directive. The argument is reported, and the
// type is still must-use.
//
//effectgo:mustuse extra
type Extra struct{}

// MakeExtra returns an Extra.
func MakeExtra() Extra { return Extra{} }

// Glued has no space before its trailing comment, so the word is not a keyword.
//
//effectgo:mustuse//note
type Glued struct{}

// Kind is an enum. Its sealed directive has a trailing comment.
//
//effectgo:sealed // every value is listed below
type Kind int

const (
	KA Kind = iota
	KB
)

func nameKind(k Kind) string {
	switch k { // missing KB
	case KA:
		return "a"
	}
	return ""
}

// Mode is an enum. Its sealed directive has an argument, which is reported; the enum is
// still sealed.
//
//effectgo:sealed extra
type Mode int

const (
	MA Mode = iota
	MB
)

func nameMode(m Mode) string {
	switch m { // missing MB
	case MA:
		return "a"
	}
	return ""
}

// Base only embeds Reader. Its type-level contract covers the methods it declares, and
// it declares none.
//
//effectgo:pure
type Base interface{ Reader }

// Reader is an interface with a method and no contract.
type Reader interface{ Read() int }

// Embedded only embeds Reader, and its type-level errors contract covers nothing.
//
//effectgo:errors ErrX
type Embedded interface{ Reader }

// Plain has a trailing comment after its pure directive, which is not an argument.
//
//effectgo:pure // checked by CI
func Plain() int { return 1 }

func use() {
	MakeNoted() // discarded: noted
	MakeExtra() // discarded: extra
}

// Tabbed has a tab before its trailing comment, which is not an argument. The function
// carries the directive, so mustuse reports that it checks nothing, once.
//
//effectgo:mustuse	// note
func Tabbed() int { return 1 }
