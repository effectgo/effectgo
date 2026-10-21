package proxy

import (
	"context"
	"net/http"

	"github.com/effectgo/effectgo/rt/web"
)

type ProxyInput struct {
	Status int `path:"status" json:"status"`
}

type ProxySuccess struct {
	Name string `json:"name"`
}

// Provider failures have their own wire shape and no constant StatusCode method.
type ProxyFailure struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type ProxyResult = web.ProxyResponse[ProxySuccess, ProxyFailure]

func Proxy(_ context.Context, in ProxyInput) (ProxyResult, error) {
	if in.Status == http.StatusOK {
		return ProxyResult{Body: ProxySuccess{Name: "ok"}}, nil
	}
	return ProxyResult{Status: in.Status, Failure: ProxyFailure{Message: "provider failure", Code: in.Status}}, nil
}

func Created(context.Context, web.NoInput) (web.ProxyResponse[ProxySuccess, ProxyFailure], error) {
	return web.ProxyResponse[ProxySuccess, ProxyFailure]{Body: ProxySuccess{Name: "created"}}, nil
}

func Register(r *web.Router) {
	web.Get(r, "/proxy/{status}", Proxy)
	web.Post(r, "/proxy-created", Created, web.SuccessStatus(http.StatusCreated))
}
