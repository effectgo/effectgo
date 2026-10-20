// Package web is the testdata stand-in for rt/web: the same route functions and
// input types as the real package (rt/web/web.go), without bodies.
package web

import "context"

// Router is the route table.
type Router struct{}

// NoInput is the input of a handler that takes nothing.
type NoInput struct{}

// NoContent is the output of a handler that answers 204.
type NoContent struct{}

type File struct {
	Filename    string
	ContentType string
	Data        []byte
}

type RouteOption struct{}

func JSONBody() RouteOption      { return RouteOption{} }
func JSONBodyEmpty() RouteOption { return RouteOption{} }
func JSONBodyExact() RouteOption { return RouteOption{} }

type Response[T any] struct {
	Body   T
	Status int
}
type FileResponse struct{ Body any }
type StreamResponse struct{ Stream any }
type RedirectResponse struct{ Location string }

// Get registers a GET route.
func Get[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error), options ...RouteOption) {
}

// Post registers a POST route.
func Post[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error), options ...RouteOption) {
}

// Put registers a PUT route.
func Put[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error), options ...RouteOption) {
}

// Patch registers a PATCH route.
func Patch[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error), options ...RouteOption) {
}

// Delete registers a DELETE route.
func Delete[In, Out any](r *Router, pattern string, h func(context.Context, In) (Out, error), options ...RouteOption) {
}
