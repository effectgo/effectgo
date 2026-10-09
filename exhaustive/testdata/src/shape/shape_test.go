package shape

// Hexagon is a variant declared only in a test file. It is not in the sealed
// set: the fact of Shape above lists the variants of the non-test files, and the
// switch here is checked against the same set.
type Hexagon struct{}

func (Hexagon) area() float64 { return 7 }

func inTest(s Shape) float64 {
	switch s.(type) { // want "missing cases in type switch on Shape: Box, Circle, Square, Triangle"
	case Hexagon:
		return 1
	}
	return 0
}
