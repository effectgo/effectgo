package metadata

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

type Output struct {
	OK bool `json:"ok"`
}

func Body(context.Context, web.NoInput) (Output, error)         { return Output{}, nil }
func Empty(context.Context, web.NoInput) (web.NoContent, error) { return web.NoContent{}, nil }
func Redirect(context.Context, web.NoInput) (web.RedirectResponse, error) {
	return web.RedirectResponse{}, nil
}
func Proxy(context.Context, web.NoInput) (web.ProxyResponse[Output, Output], error) {
	return web.ProxyResponse[Output, Output]{}, nil
}
func Register(r *web.Router) {
	code := 201
	web.Get(r, "/variable", Body, web.SuccessStatus(code))
	web.Get(r, "/invalid", Body, web.SuccessStatus(400))
	web.Get(r, "/body204", Body, web.SuccessStatus(204))
	web.Get(r, "/empty200", Empty, web.SuccessStatus(200))
	web.Get(r, "/redirect200", Redirect, web.SuccessStatus(200))
	web.Get(r, "/duplicate", Body, web.SuccessStatus(201), web.SuccessStatus(202))
	web.Get(r, "/range", Body, web.SuccessStatusRange(299, 200))
	web.Get(r, "/static", Body, web.SuccessStatuses(200, 201))
	web.Get(r, "/proxy-alternatives", Proxy, web.SuccessStatuses(200, 206))
	web.Get(r, "/proxy-range", Proxy, web.SuccessStatusRange(200, 299))
}
