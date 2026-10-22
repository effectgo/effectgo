package bad

func Value(pointer *int) int {
	value := *pointer
	if pointer == nil {
		return 0
	}
	return value
}
