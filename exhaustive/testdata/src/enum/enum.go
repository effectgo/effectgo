package enum

// Status is a sealed enum: its variants are the distinct values of the
// constants of this type.
//
//effectgo:sealed
type Status int // want Status:"enum\\(AnyStatus,Archived,Done,Legacy,Open,hidden\\)"

const (
	AnyStatus Status = iota
	Open
	Done
	Archived
)

// Legacy has the value of Done: a switch that covers Done covers Legacy too.
const Legacy = Done

// hidden is unexported. Only this package can name it, so a switch elsewhere
// does not have to cover it.
const hidden Status = 9

// Label misses Archived.
func Label(s Status) string {
	switch s { // want "missing cases in switch on Status: Archived, hidden"
	case AnyStatus:
		return "any"
	case Open:
		return "open"
	case Done:
		return "done"
	}
	return ""
}

// Full names every value, the unexported one included.
func Full(s Status) string {
	switch s {
	case AnyStatus, Open, Done, Archived, hidden:
		return "all"
	}
	return ""
}

// ByAlias covers the value of Done through Legacy, and misses the rest.
func ByAlias(s Status) string {
	switch s { // want "missing cases in switch on Status: AnyStatus, Archived, Open, hidden"
	case Legacy:
		return "done"
	}
	return ""
}

// Defaulted has a default clause.
func Defaulted(s Status) string {
	switch s {
	case Open:
		return "open"
	default:
		return ""
	}
}

// NonConstant has a case that is a variable: the switch is not decided.
func NonConstant(s, x Status) string {
	switch s {
	case x:
		return "x"
	}
	return ""
}

// InPackage can name the hidden value, so it must be covered here.
func InPackage(s Status) string {
	switch s { // want "missing cases in switch on Status: AnyStatus, Archived, Open"
	case Done, hidden:
		return "done"
	}
	return ""
}

// Level is a sealed string enum.
//
//effectgo:sealed
type Level string // want Level:"enum\\(High,Low,none\\)"

const (
	Low  Level = "low"
	High Level = "high"
	none Level = ""
)

// Shape is not an enum: the directive does not apply to a struct.
//
//effectgo:sealed
type Shape struct{ n int } // want "applies to interfaces and to integer or string types"

// Plain has no directive, so it is never checked.
type Plain int

const (
	P1 Plain = iota
	P2
)

func PlainSwitch(p Plain) int {
	switch p {
	case P1:
		return 1
	}
	return 0
}
