// Package ifacedef declares interfaces with error contracts. It is a
// dependency of the test packages, so its facts are exported but not checked.
package ifacedef

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// Status is an error with a status code, like fuego.ErrorWithStatus.
type Status interface {
	error
	StatusCode() int
}

// Getter: the type-level contract applies to Get.
//
//effectgo:errors ErrNotFound, iface:Status
type Getter interface {
	Get(id int) error
}

// Putter: a contract on the method itself.
type Putter interface {
	//effectgo:errors ErrNotFound, ErrConflict
	Put(id int, v string) error
}

// Mixed: the type-level contract applies to Get; Put overrides it.
//
//effectgo:errors ErrNotFound
type Mixed interface {
	Get(id int) error
	//effectgo:errors ErrConflict
	Put(id int, v string) error
}

// Loader: an opaque error is allowed.
type Loader interface {
	//effectgo:errors ?
	Load(name string) error
}

// Plain has no contract.
type Plain interface {
	Do() error
}
