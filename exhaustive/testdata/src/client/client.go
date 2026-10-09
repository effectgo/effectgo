package client

import "shape"

// Cross-package: the variants come from the shape package's fact.
func Describe(s shape.Shape) string {
	switch s.(type) { // want "missing cases in type switch on Shape: Square, Triangle"
	case shape.Circle:
		return "circle"
	}
	return ""
}

func Full(s shape.Shape) string {
	switch s.(type) {
	case shape.Circle, *shape.Square, shape.Triangle:
		return "all"
	}
	return ""
}
