package api

import (
	"net/http"

	"github.com/faintghost/effectgo/examples/todo/domain"
	"github.com/faintghost/effectgo/examples/todo/httpres"
	"github.com/faintghost/effectgo/examples/todo/result"
	"github.com/go-fuego/fuego"
)

func registerV2(s *fuego.Server, svc *domain.Service) {
	fuego.Post(s, "/v2/todos", httpres.WithBody(v2Create(svc)), docOpts("v2CreateTodo", fuego.OptionDefaultStatusCode(http.StatusCreated))...)
	fuego.Get(s, "/v2/todos", httpres.NoBody(v2List(svc)), docOpts("v2ListTodos")...)
	fuego.Get(s, "/v2/todos/{id}", httpres.NoBody(v2Get(svc)), docOpts("v2GetTodo")...)
	fuego.Put(s, "/v2/todos/{id}", httpres.WithBody(v2Update(svc)), docOpts("v2UpdateTodo")...)
	fuego.Delete(s, "/v2/todos/{id}", httpres.NoBody(v2Delete(svc)), docOpts("v2DeleteTodo")...)
}

func v2Create(svc *domain.Service) func(fuego.ContextWithBody[createReq]) result.Result[domain.Todo] {
	return func(c fuego.ContextWithBody[createReq]) result.Result[domain.Todo] {
		return result.AndThen(result.From(c.Body()), func(b createReq) result.Result[domain.Todo] {
			return svc.CreateR(b.Title)
		})
	}
}

func v2List(svc *domain.Service) func(fuego.ContextNoBody) result.Result[[]domain.Todo] {
	return func(c fuego.ContextNoBody) result.Result[[]domain.Todo] {
		return svc.ListR()
	}
}

func v2Get(svc *domain.Service) func(fuego.ContextNoBody) result.Result[domain.Todo] {
	return func(c fuego.ContextNoBody) result.Result[domain.Todo] {
		return result.AndThen(result.From(parseID(c)), svc.GetR)
	}
}

func v2Update(svc *domain.Service) func(fuego.ContextWithBody[updateReq]) result.Result[domain.Todo] {
	return func(c fuego.ContextWithBody[updateReq]) result.Result[domain.Todo] {
		return result.AndThen(result.From(parseID(c)), func(id int) result.Result[domain.Todo] {
			return result.AndThen(result.From(c.Body()), func(b updateReq) result.Result[domain.Todo] {
				return svc.UpdateR(id, b.Title, b.Done)
			})
		})
	}
}

func v2Delete(svc *domain.Service) func(fuego.ContextNoBody) result.Result[deleted] {
	return func(c fuego.ContextNoBody) result.Result[deleted] {
		r := result.AndThen(result.From(parseID(c)), svc.DeleteR)
		return result.Map(r, func(struct{}) deleted { return deleted{} })
	}
}
