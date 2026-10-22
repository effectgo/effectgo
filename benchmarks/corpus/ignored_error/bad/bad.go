package bad

import "os"

func Remove(path string) {
	os.Remove(path)
}
