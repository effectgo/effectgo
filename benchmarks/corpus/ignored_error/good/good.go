package good

import "os"

func Remove(path string) error {
	return os.Remove(path)
}
