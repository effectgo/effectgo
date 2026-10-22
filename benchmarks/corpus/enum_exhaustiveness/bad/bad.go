package bad

//effectgo:sealed
type State int

const (
	Pending State = iota
	Complete
)

func Label(state State) string {
	//exhaustive:enforce
	switch state {
	case Pending:
		return "pending"
	}
	return "unknown"
}
