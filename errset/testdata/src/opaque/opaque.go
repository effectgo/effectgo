package opaque

import (
	"httpres"
	"strconv"
)

var _ = httpres.ToHTTPError

//effectgo:handler httpres.ToHTTPError
func v3Parse() func(string) error { // want "handler v3Parse may return an unclassified error \\(via strconv.Atoi\\), which errmap httpres.ToHTTPError does not map" v3Parse:"."
	return func(s string) error {
		_, err := strconv.Atoi(s)
		return err
	}
}
