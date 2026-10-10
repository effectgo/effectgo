package args

// Kind is an enum. Its directive has a trailing comment, which is not an argument.
//
//effectgo:sealed // the constants below are all of its values
type Kind int // want Kind:"."

const (
	A Kind = iota
	B
)

func name(k Kind) string {
	switch k { // want `missing cases in switch on Kind: B`
	case A:
		return "a"
	}
	return ""
}

// Mode is an enum. Its directive has an argument, which is reported; the enum is still
// sealed.
//
//effectgo:sealed extra // want `//effectgo:sealed takes no arguments`
type Mode int // want Mode:"."

const (
	X Mode = iota
	Y
)

func mode(m Mode) string {
	switch m { // want `missing cases in switch on Mode: Y`
	case X:
		return "x"
	}
	return ""
}
