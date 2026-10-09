package client

import "shape"

// Cross-package: the variants come from the shape package's fact.
func Describe(s shape.Shape) string {
	switch s.(type) { // want "missing cases in type switch on Shape: Box, Square, Triangle"
	case shape.Circle:
		return "circle"
	}
	return ""
}

func Full(s shape.Shape) string {
	switch s.(type) {
	case shape.Circle, *shape.Square, shape.Triangle, shape.Box[float64]:
		return "all"
	}
	return ""
}

func Drawn(d shape.Drawable) string {
	switch d.(type) { // want "missing cases in type switch on Drawable: Square"
	case shape.Circle:
		return "c"
	}
	return ""
}
