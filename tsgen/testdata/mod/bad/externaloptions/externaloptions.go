package externaloptions

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

func Read(context.Context, web.NoInput) (web.NoContent, error) {
	return web.NoContent{}, nil
}

func indirect() web.RouteOption { return web.JSONBody() }

func Register(r *web.Router) {
	option := web.JSONBody()
	web.Get(r, "/opaque", Read, option)
	web.Get(r, "/indirect", Read, indirect())
}
