package good

import "fmt"

func Print(value int) (int, error) {
	return fmt.Printf("%d\n", value)
}
