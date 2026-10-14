// Package rtshared is a handler's package, other than the routes: the route check
// reads the error set of Find from the fact that errset exports for it.
package rtshared

import (
	"context"
	"errors"
)

// ErrMissing is a sentinel: a plain error with no status of its own.
var ErrMissing = errors.New("missing")

type Input struct{ ID int64 }

type Output struct{}

// Find returns ErrMissing for the zero id.
func Find(ctx context.Context, in Input) (Output, error) {
	if in.ID == 0 {
		return Output{}, ErrMissing
	}
	return Output{}, nil
}
