// Package rtfake has the shape of rt/web's route functions: a router, a
// pattern, and a generic handler func(context.Context, In) (Out, error). The
// router is argument 0, so the pattern is argument 1 and the handler argument 2.
package rtfake

import "context"

type Router struct{}

func Get[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error)) {}

func Post[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error)) {}
