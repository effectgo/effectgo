package e

import "os"

// Under -mustuse.allow=(*os.File).Close the calls of the method value are
// allowed, the same as a direct call of the method.
func Use(f *os.File, g func() error) {
	f.Close()
	closer := f.Close
	closer()
	var h func() error = f.Close
	h()
	m := f.Close
	m = g
	m() // want "result of m is discarded"
}
