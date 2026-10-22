package bad

//effectgo:sealed
type Node interface {
	seal()
}

type Text struct{}

type Number struct{}

func (*Text) seal() {}

func (*Number) seal() {}

func Kind(node Node) string {
	//exhaustive:enforce
	switch node.(type) {
	case *Text:
		return "text"
	}
	return "unknown"
}
