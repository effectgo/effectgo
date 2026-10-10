package wrap

import "shape"

// Wrapped embeds a sealed interface of another package. It has no unexported
// method of its own, so its variants are those of shape.Shape.
type Wrapped interface {
	shape.Shape
}

func Check(w Wrapped) int {
	switch w.(type) { // want "missing cases in type switch on Wrapped: Box, Square, Triangle"
	case shape.Circle:
		return 1
	}
	return 0
}
