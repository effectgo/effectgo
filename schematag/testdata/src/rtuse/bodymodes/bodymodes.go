// Package bodymodes covers runtime-backed request and response modes without
// treating HTTP metadata or explicit encoding/json inputs as schema failures.
package bodymodes

import (
	"context"
	"github.com/faintghost/effectgo/rt/web"
)

type Input struct {
	ID      int    `path:"id" json:"id"`
	Payload []byte `json:"payload"`
}
type Output struct {
	OK bool `json:"ok"`
}
type Base struct {
	ID     int      `json:"id"`
	Shadow chan int `json:"shadow"`
}
type Detail struct {
	Base
	Shadow string `json:"shadow"`
	Rows   []struct {
		SKU string `json:"sku"`
	} `json:"rows"`
	ignored chan int `schema:"unknown"`
}
type Ambiguous struct {
	Left  int    `json:"same"` // want `ambiguous output json name "same"`
	Right string `json:"same"`
}

func Inherited(context.Context, web.NoInput) (Detail, error)    { return Detail{}, nil }
func Duplicate(context.Context, web.NoInput) (Ambiguous, error) { return Ambiguous{}, nil }

type HeaderInput struct {
	Token string `header:"X-Token" json:"token"`
}
type UpdateInput struct {
	Name string `json:"name"`
}
type WrappedAlias = web.Response[Output]
type FileAlias = web.FileResponse

func Update(context.Context, Input) (Output, error)              { return Output{}, nil }
func Remove(context.Context, UpdateInput) (web.NoContent, error) { return web.NoContent{}, nil }
func Header(context.Context, HeaderInput) (Output, error)        { return Output{}, nil }
func Wrapped(context.Context, web.NoInput) (WrappedAlias, error) { return WrappedAlias{}, nil }
func Download(context.Context, web.NoInput) (FileAlias, error)   { return FileAlias{}, nil }
func Stream(context.Context, web.NoInput) (web.StreamResponse, error) {
	return web.StreamResponse{}, nil
}
func Redirect(context.Context, web.NoInput) (web.RedirectResponse, error) {
	return web.RedirectResponse{}, nil
}

func Register(r *web.Router) {
	web.Post(r, "/update/{id}", Update, web.JSONBody())
	web.Post(r, "/exact/{id}", Update, web.JSONBodyExact())
	web.Delete(r, "/remove", Remove)
	web.Get(r, "/header", Header)
	web.Get(r, "/wrapped", Wrapped)
	web.Get(r, "/download", Download)
	web.Get(r, "/stream", Stream)
	web.Get(r, "/redirect", Redirect)
	web.Get(r, "/inherited", Inherited)
	web.Get(r, "/duplicate", Duplicate)
}
