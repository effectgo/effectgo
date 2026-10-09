package shape

// Shape is sealed by its unexported marker method.
type Shape interface { // want Shape:"sealed\\(Box,Circle,Square,Triangle\\)"
	area() float64
}

type Circle struct{ R float64 }

func (Circle) area() float64 { return 3 }

type Square struct{ S float64 }

func (*Square) area() float64 { return 4 }

// Triangle is a variant too; the switches below must name it.
type Triangle struct{}

func (Triangle) area() float64 { return 5 }

// Box is a generic variant: every instance is the variant Box.
type Box[T any] struct{ V T }

func (Box[T]) area() float64 { return 6 }

// Alias names a variant. A case on the alias covers Circle.
type Alias = Circle

// ShapeAlias names the sealed interface. A switch on it is checked too.
type ShapeAlias = Shape

// Missing lacks Box, Square and Triangle.
func Missing(s Shape) float64 {
	switch s.(type) { // want "missing cases in type switch on Shape: Box, Square, Triangle"
	case Circle:
		return 1
	}
	return 0
}

// Complete names every variant: the alias, the pointer form and the generic.
func Complete(s Shape) float64 {
	switch v := s.(type) {
	case Alias:
		return v.R
	case *Square:
		return 2
	case Triangle:
		return 3
	case Box[int], Box[string]:
		return 4
	}
	return 0
}

// ViaAlias switches on the alias of the interface.
func ViaAlias(s ShapeAlias) float64 {
	switch s.(type) { // want "missing cases in type switch on Shape: Box, Square, Triangle"
	case Circle:
		return 1
	}
	return 0
}

// Parenthesized and dereferenced operands are checked like plain ones.
func Parenthesized(s Shape) float64 {
	switch (s).(type) { // want "missing cases in type switch on Shape: Box, Square, Triangle"
	case Circle:
		return 1
	}
	return 0
}

func Dereferenced(p *Shape) float64 {
	switch (*p).(type) { // want "missing cases in type switch on Shape: Box, Square, Triangle"
	case Circle:
		return 1
	}
	return 0
}

// Interface cases cover every variant that implements them.
func ByInterface(s Shape) float64 {
	switch s.(type) {
	case interface{ area() float64 }:
		return 1
	}
	return 0
}

// Drawable embeds Shape and adds Draw: only Circle and Square implement it.
type Drawable interface { // want Drawable:"sealed\\(Circle,Square\\)"
	Shape
	Draw()
}

func (Circle) Draw()  {}
func (*Square) Draw() {}

func DrawMissing(d Drawable) int {
	switch d.(type) { // want "missing cases in type switch on Drawable: Square"
	case Circle:
		return 1
	}
	return 0
}

// Solid embeds Shape and adds nothing: every variant implements it.
type Solid interface { // want Solid:"sealed\\(Box,Circle,Square,Triangle\\)"
	Shape
}

func SolidMissing(s Solid) int {
	switch s.(type) { // want "missing cases in type switch on Solid: Box, Circle, Square, Triangle"
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
