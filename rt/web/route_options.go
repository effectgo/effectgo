package web

import (
	"fmt"
	"reflect"

	"github.com/faintghost/effectgo/rt/schema"
)

// RouteOption declares additional HTTP boundary contracts.
// Its zero value is invalid.
type RouteOption struct {
	validate func() error
}

// ExternalError declares an error response written outside the typed handler,
// such as an existing authentication or module middleware, or written by a custom
// WithErrorEncoder. E describes the exact JSON body and has a constant StatusCode
// method. It does not require a wire _tag: the generated client adds a
// discriminant to the decoded error instance. A declaration for an inferred
// handler error overrides that error's default tagged wire contract.
// Register these calls directly on each route so the generator can read E.
// This validates the declared schema at startup; it cannot prove middleware
// actually writes that body, which requires a boundary integration check.
func ExternalError[E interface {
	error
	StatusCode() int
}]() RouteOption {
	return RouteOption{validate: func() error {
		t := reflect.TypeFor[E]()
		if t.Kind() != reflect.Struct {
			return fmt.Errorf("%v must be a struct error with a value StatusCode method", t)
		}
		var e E
		if status := e.StatusCode(); status < 400 || status > 599 {
			return fmt.Errorf("%v has invalid error status %d", t, status)
		}
		return schema.Compile[E]()
	}}
}
