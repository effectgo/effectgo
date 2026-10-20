package requestboundary

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

type Item struct {
	Name string  `json:"name"`
	Note *string `json:"note"`
}

type Input struct {
	ID      int                `path:"id" json:"id"`
	Limit   *int               `query:"limit" json:"limit"`
	Name    string             `json:"name"`
	Count   int32              `json:"count" schema:"min=0"`
	Enabled bool               `json:"enabled"`
	Note    *string            `json:"note"`
	Nested  Item               `json:"nested"`
	Items   []Item             `json:"items"`
	Values  map[string]*string `json:"values"`
	Opaque  any                `json:"opaque"`
}

type BodyInput struct {
	Items []Item `json:"items"`
}

type Output struct {
	OK bool `json:"ok"`
}

func Strict(_ context.Context, _ Input) (Output, error)       { return Output{}, nil }
func Legacy(_ context.Context, _ Input) (Output, error)       { return Output{}, nil }
func Empty(_ context.Context, _ Input) (Output, error)        { return Output{}, nil }
func BodyOnly(_ context.Context, _ BodyInput) (Output, error) { return Output{}, nil }

func Register(r *web.Router) {
	web.Post(r, "/strict/{id}", Strict)
	web.Post(r, "/legacy/{id}", Legacy, web.JSONBody())
	web.Post(r, "/empty/{id}", Empty, web.JSONBodyEmpty())
	web.Post(r, "/body", BodyOnly)
}
