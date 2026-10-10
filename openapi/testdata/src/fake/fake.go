// Package fake has the shape of Fuego's route functions: a server, a path, a
// generic controller and options.
package fake

type Server struct{}

type Ctx[B, P any] struct{}

func Get[T, B, P any](s *Server, path string, controller func(Ctx[B, P]) (T, error), options ...any) {
}

func Post[T, B, P any](s *Server, path string, controller func(Ctx[B, P]) (T, error), options ...any) {
}

func Put[T, B, P any](s *Server, path string, controller func(Ctx[B, P]) (T, error), options ...any) {
}

func Delete[T, B, P any](s *Server, path string, controller func(Ctx[B, P]) (T, error), options ...any) {
}
