package multipart

import (
	"context"
	"github.com/effectgo/effectgo/rt/web"
)

type Good struct {
	ID    int       `path:"id" json:"id"`
	File  web.File  `form:"file" json:"-"`
	Extra *web.File `form:"extra" json:"-"`
}
type Mixed struct {
	File web.File `form:"file" json:"-"`
	Name string   `json:"name"`
}
type Scalar struct {
	Name string `form:"name" json:"-"` // want `multipart fields require .*matching json name`
}
type Duplicate struct {
	First  web.File `form:"file" json:"-"`
	Second web.File `form:"file" json:"-"`
}

func Read(context.Context, Good) (web.NoContent, error)       { return web.NoContent{}, nil }
func Mix(context.Context, Mixed) (web.NoContent, error)       { return web.NoContent{}, nil }
func Text(context.Context, Scalar) (web.NoContent, error)     { return web.NoContent{}, nil }
func Twice(context.Context, Duplicate) (web.NoContent, error) { return web.NoContent{}, nil }
func Routes(r *web.Router) {
	web.Post(r, "/upload/{id}", Read)
	web.Get(r, "/upload/{id}", Read) // want `GET takes no body, but the input has form fields`
	web.Post(r, "/mixed", Mix)       // want `multipart file fields cannot be mixed with JSON body fields`
	web.Post(r, "/scalar", Text)
	web.Post(r, "/duplicate", Twice) // want `multipart form name file is repeated`
}
