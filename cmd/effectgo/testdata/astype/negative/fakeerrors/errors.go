package errors

// AsType deliberately matches nothing, regardless of its type argument.
func AsType[T error](err error) (T, bool) {
	var zero T
	return zero, false
}
