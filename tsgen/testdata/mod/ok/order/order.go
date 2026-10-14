// Package order has a handler whose error variants share a status: the error list
// is sorted by status, then by name.
package order

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

// Bad is a 400.
type Bad struct{}

// Error implements error.
func (Bad) Error() string { return "bad" }

// StatusCode is 400.
func (Bad) StatusCode() int { return 400 }

// ZNotFound is a 404.
type ZNotFound struct{}

// Error implements error.
func (ZNotFound) Error() string { return "z" }

// StatusCode is 404.
func (ZNotFound) StatusCode() int { return 404 }

// ANotFound is a 404 whose name sorts first.
type ANotFound struct{}

// Error implements error.
func (ANotFound) Error() string { return "a" }

// StatusCode is 404.
func (ANotFound) StatusCode() int { return 404 }

// In is the path of Run.
type In struct {
	ID int64 `path:"id" json:"id"`
}

// Out is the response.
type Out struct {
	OK bool `json:"ok"`
}

// Run fails in four ways.
func Run(ctx context.Context, in In) (Out, error) {
	switch in.ID {
	case 1:
		return Out{}, ZNotFound{}
	case 2:
		return Out{}, ANotFound{}
	case 3:
		return Out{}, Bad{}
	}
	return Out{OK: true}, nil
}

// Routes registers Run.
func Routes(r *web.Router) { web.Get(r, "/o/{id}", Run) }
