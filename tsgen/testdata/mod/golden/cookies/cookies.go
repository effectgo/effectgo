package cookies

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

type In struct {
	Token   string `header:"Authorization" json:"token"`
	Session string `cookie:"session" json:"session"`
	CSRF    string `cookie:"csrf" json:"csrf"`
}

type Out struct {
	OK bool `json:"ok"`
}

func Read(context.Context, In) (Out, error) { return Out{OK: true}, nil }

func Register(r *web.Router) {
	web.Get(r, "/api/cookie-auth", Read)
}
