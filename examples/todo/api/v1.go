package api

import (
	"errors"
	"net/http"

	"github.com/faintghost/effectgo/examples/todo/domain"
	"github.com/go-fuego/fuego"
)

// v1Err is the idiomatic handler's own error mapping. Errors that already
// carry an HTTP status pass through unchanged.
func v1Err(err error) error {
	var withStatus fuego.ErrorWithStatus
	if errors.As(err, &withStatus) {
		return err
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return fuego.NotFoundError{Err: err, Detail: err.Error()}
	case errors.Is(err, domain.ErrInvalid):
		return fuego.BadRequestError{Err: err, Detail: err.Error()}
	default:
		return err
	}
}

func registerV1(s *fuego.Server, svc *domain.Service) {
	fuego.Post(s, "/v1/todos", v1Create(svc), docOpts("v1CreateTodo", fuego.OptionDefaultStatusCode(http.StatusCreated))...)
	fuego.Get(s, "/v1/todos", v1List(svc), docOpts("v1ListTodos")...)
	fuego.Get(s, "/v1/todos/{id}", v1Get(svc), docOpts("v1GetTodo")...)
	fuego.Put(s, "/v1/todos/{id}", v1Update(svc), docOpts("v1UpdateTodo")...)
	fuego.Delete(s, "/v1/todos/{id}", v1Delete(svc), docOpts("v1DeleteTodo")...)
}

func v1Create(svc *domain.Service) func(fuego.ContextWithBody[createReq]) (domain.Todo, error) {
	return func(c fuego.ContextWithBody[createReq]) (domain.Todo, error) {
		body, err := c.Body()
		if err != nil {
			return domain.Todo{}, err
		}
		t, err := svc.Create(body.Title)
		if err != nil {
			return domain.Todo{}, v1Err(err)
		}
		return t, nil
	}
}

func v1List(svc *domain.Service) func(fuego.ContextNoBody) ([]domain.Todo, error) {
	return func(c fuego.ContextNoBody) ([]domain.Todo, error) {
		return svc.List(), nil
	}
}

func v1Get(svc *domain.Service) func(fuego.ContextNoBody) (domain.Todo, error) {
	return func(c fuego.ContextNoBody) (domain.Todo, error) {
		id, err := parseID(c)
		if err != nil {
			return domain.Todo{}, err
		}
		t, err := svc.Get(id)
		if err != nil {
			return domain.Todo{}, v1Err(err)
		}
		return t, nil
	}
}

func v1Update(svc *domain.Service) func(fuego.ContextWithBody[updateReq]) (domain.Todo, error) {
	return func(c fuego.ContextWithBody[updateReq]) (domain.Todo, error) {
		id, err := parseID(c)
		if err != nil {
			return domain.Todo{}, err
		}
		body, err := c.Body()
		if err != nil {
			return domain.Todo{}, err
		}
		t, err := svc.Update(id, body.Title, body.Done)
		if err != nil {
			return domain.Todo{}, v1Err(err)
		}
		return t, nil
	}
}

func v1Delete(svc *domain.Service) func(fuego.ContextNoBody) (deleted, error) {
	return func(c fuego.ContextNoBody) (deleted, error) {
		id, err := parseID(c)
		if err != nil {
			return deleted{}, err
		}
		if err := svc.Delete(id); err != nil {
			return deleted{}, v1Err(err)
		}
		return deleted{}, nil
	}
}
