package bad

import "fmt"

func Print(value string) (int, error) {
	return fmt.Printf("%d\n", value)
}
