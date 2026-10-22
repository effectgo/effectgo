package good

func Value(pointer *int) int {
	if pointer == nil {
		return 0
	}
	return *pointer
}
