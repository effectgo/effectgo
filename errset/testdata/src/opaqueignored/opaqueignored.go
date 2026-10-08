package opaqueignored

import (
	"httpres"
	"strconv"
)

var _ = httpres.ToHTTPError

//effectgo:handler httpres.ToHTTPError
func v3Parse() func(string) error { // want v3Parse:"."
	return func(s string) error {
		_, err := strconv.Atoi(s)
		return err
	}
}
