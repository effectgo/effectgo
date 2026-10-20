// Package rtlib holds the handlers that the rtweb package registers. Its
// diagnostics are not checked by the test: rtweb is the package under test, and
// its facts are the ones the route check reads.
package rtlib

import (
	"context"
	"errors"
)

type In struct{ ID int64 }

type Out struct{ ID int64 }

// NotFound has a constant StatusCode.
type NotFound struct{}

func (NotFound) Error() string   { return "not found" }
func (NotFound) StatusCode() int { return 404 }

// Invalid has a constant StatusCode.
type Invalid struct{ Field string }

func (Invalid) Error() string   { return "invalid" }
func (Invalid) StatusCode() int { return 400 }

// Dyn's status depends on its field: it is not a constant.
type Dyn struct{ Code int }

func (Dyn) Error() string     { return "dyn" }
func (d Dyn) StatusCode() int { return d.Code }

// Plain has no StatusCode method.
type Plain struct{}

func (Plain) Error() string { return "plain" }

// ErrSentinel is a sentinel: a plain error with no status of its own.
var ErrSentinel = errors.New("sentinel")

// ErrTyped is a sentinel of a type with a StatusCode method: its status is that
// of its initializer, which is not known, so it is not reported.
var ErrTyped = NotFound{}

// Source is an interface without an effectgo contract: a call through it has an
// unknown origin.
type Source interface{ Next() error }

var src Source

// GetOK returns only types with constant statuses.
func GetOK(ctx context.Context, in In) (Out, error) {
	if in.ID == 0 {
		return Out{}, NotFound{}
	}
	if in.ID < 0 {
		return Out{}, Invalid{Field: "id"}
	}
	return Out{ID: in.ID}, nil
}

// GetSentinel returns a sentinel.
func GetSentinel(ctx context.Context, in In) (Out, error) { return Out{}, ErrSentinel }

// GetFresh returns a fresh error created by errors.New.
func GetFresh(ctx context.Context, in In) (Out, error) { return Out{}, errors.New("fresh") }

// GetDyn returns a type whose StatusCode is not constant.
func GetDyn(ctx context.Context, in In) (Out, error) { return Out{}, Dyn{Code: int(in.ID)} }

// GetPlain returns a type without StatusCode.
func GetPlain(ctx context.Context, in In) (Out, error) { return Out{}, Plain{} }

// GetTyped returns the sentinel whose type has a StatusCode: not reported.
func GetTyped(ctx context.Context, in In) (Out, error) { return Out{}, ErrTyped }

// GetOpaque returns an error of unknown origin (a call through Source): not
// reported without a contract.
func GetOpaque(ctx context.Context, in In) (Out, error) { return Out{}, src.Next() }

// GetContract declares its error contract, so its unknown origin is reported.
//
//effectgo:errors ?
func GetContract(ctx context.Context, in In) (Out, error) { return Out{}, src.Next() }

// Store has handler methods.
type Store struct{}

// Update returns a sentinel.
func (s *Store) Update(ctx context.Context, in In) (Out, error) { return Out{}, ErrSentinel }

// Read returns only constant statuses.
func (s *Store) Read(ctx context.Context, in In) (Out, error) { return Out{}, NotFound{} }
