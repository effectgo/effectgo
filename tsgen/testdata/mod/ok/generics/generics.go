package generics

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

type Page[T any] struct {
	Items []T `json:"items"`
}
type Envelope[T any] struct {
	Data T `json:"data"`
}
type First struct {
	Name string `json:"name"`
}
type Second struct {
	Count int `json:"count"`
}

func Names(context.Context, web.NoInput) (Envelope[Page[First]], error) {
	return Envelope[Page[First]]{}, nil
}
func Counts(context.Context, web.NoInput) (Envelope[Page[Second]], error) {
	return Envelope[Page[Second]]{}, nil
}
func Register(r *web.Router) {
	web.Get(r, "/names", Names)
	web.Get(r, "/counts", Counts)
}
