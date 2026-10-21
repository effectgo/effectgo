package cookies

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

type In struct {
	Session string `cookie:"session" json:"session"`
	CSRF    string `cookie:"csrf" json:"csrf"`
}

type Out struct{}

func Get(context.Context, In) (Out, error) { return Out{}, nil }

func Routes(r *web.Router) {
	web.Get(r, "/cookies", Get)
}
