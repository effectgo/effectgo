// Package astypemap is an errmap through errors.AsType. The handler check of the
// importing package reads its mapped variants.
package astypemap

import "errors"

type BusyError struct{}

func (*BusyError) Error() string { return "busy" }

// Mapper maps BusyError, through errors.AsType in an if statement.
//
//effectgo:errmap
func Mapper(err error) error {
	if _, ok := errors.AsType[*BusyError](err); ok {
		return err
	}
	return err
}
