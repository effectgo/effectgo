package api

import (
	"github.com/faintghost/effectgo/examples/todo/domain"
	"github.com/faintghost/effectgo/examples/todo/httpres"
	"github.com/go-fuego/fuego"
)

// Keeps the httpres import (used by the //effectgo:handler directives below) compiled.
var _ = httpres.ToHTTPError

// registerV3 registers the idiomatic baseline: (T, error) handlers with no per-handler error
// mapping. Domain errors become HTTP errors once, in the server-wide error
// handler (see errorHandler in common.go).
func registerV3(s *fuego.Server, svc *domain.Service) {
	fuego.Post(s, "/v3/todos", v3Create(svc), docOpts("v3CreateTodo", fuego.OptionDefaultStatusCode(201))...)
	fuego.Get(s, "/v3/todos", v3List(svc), docOpts("v3ListTodos")...)
	fuego.Get(s, "/v3/todos/{id}", v3Get(svc), docOpts("v3GetTodo")...)
	fuego.Put(s, "/v3/todos/{id}", v3Update(svc), docOpts("v3UpdateTodo")...)
	fuego.Delete(s, "/v3/todos/{id}", v3Delete(svc), docOpts("v3DeleteTodo")...)
}

//effectgo:handler httpres.ToHTTPError
func v3Create(svc *domain.Service) func(fuego.ContextWithBody[createReq]) (domain.Todo, error) {
	return func(c fuego.ContextWithBody[createReq]) (domain.Todo, error) {
		body, err := c.Body()
		if err != nil {
			return domain.Todo{}, err
		}
		return svc.Create(body.Title)
	}
}

//effectgo:handler httpres.ToHTTPError
func v3List(svc *domain.Service) func(fuego.ContextNoBody) ([]domain.Todo, error) {
	return func(c fuego.ContextNoBody) ([]domain.Todo, error) {
		return svc.List(), nil
	}
}

//effectgo:handler httpres.ToHTTPError
func v3Get(svc *domain.Service) func(fuego.ContextNoBody) (domain.Todo, error) {
	return func(c fuego.ContextNoBody) (domain.Todo, error) {
		id, err := parseID(c)
		if err != nil {
			return domain.Todo{}, err
		}
		return svc.Get(id)
	}
}

//effectgo:handler httpres.ToHTTPError
func v3Update(svc *domain.Service) func(fuego.ContextWithBody[updateReq]) (domain.Todo, error) {
	return func(c fuego.ContextWithBody[updateReq]) (domain.Todo, error) {
		id, err := parseID(c)
		if err != nil {
			return domain.Todo{}, err
		}
		body, err := c.Body()
		if err != nil {
			return domain.Todo{}, err
		}
		return svc.Update(id, body.Title, body.Done)
	}
}

//effectgo:handler httpres.ToHTTPError
func v3Delete(svc *domain.Service) func(fuego.ContextNoBody) (deleted, error) {
	return func(c fuego.ContextNoBody) (deleted, error) {
		id, err := parseID(c)
		if err != nil {
			return deleted{}, err
		}
		return deleted{}, svc.Delete(id)
	}
}
