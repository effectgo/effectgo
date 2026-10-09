package shape

// Shape is sealed by its unexported marker method.
type Shape interface { // want Shape:"sealed\\(Circle,Square,Triangle\\)"
	area() float64
}

type Circle struct{ R float64 }

func (Circle) area() float64 { return 3 }

type Square struct{ S float64 }

func (*Square) area() float64 { return 4 }

// Triangle is a variant too; the switches below must name it.
type Triangle struct{}

func (Triangle) area() float64 { return 5 }

// Missing lacks Square and Triangle.
func Missing(s Shape) float64 {
	switch s.(type) { // want "missing cases in type switch on Shape: Square, Triangle"
	case Circle:
		return 1
	}
	return 0
}

// Complete names every variant, the pointer form included.
func Complete(s Shape) float64 {
	switch v := s.(type) {
	case Circle:
		return v.R
	case *Square:
		return 2
	case Triangle:
		return 3
	}
	return 0
}

// Defaulted has a default clause, which makes it exhaustive.
func Defaulted(s Shape) float64 {
	switch s.(type) {
	case Circle:
		return 1
	default:
		return 0
	}
}

// Sealed is an explicit directive on an interface with an unexported method.
//
//effectgo:sealed
type Sealed interface { // want Sealed:"sealed\\(OnlyOne\\)"
	seal()
}

type OnlyOne struct{}

func (OnlyOne) seal() {}

func OneOfOne(s Sealed) int {
	switch s.(type) { // want "missing cases in type switch on Sealed"
	}
	return 0
}

// Open is an ordinary interface: no unexported method, so nothing is checked.
type Open interface {
	Name() string
}

type A struct{}

func (A) Name() string { return "a" }

func OpenSwitch(o Open) int {
	switch o.(type) {
	case A:
		return 1
	}
	return 0
}
