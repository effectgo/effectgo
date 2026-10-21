package upload

import (
	"context"
	"github.com/effectgo/effectgo/rt/web"
)

type Mixed struct {
	File web.File `form:"file" json:"-"`
	Name string   `json:"name"`
}
type Duplicate struct {
	First  web.File  `form:"file" json:"-"`
	Second *web.File `form:"file" json:"-"`
}
type Scalar struct {
	Name string `form:"name" json:"-"`
}

func MixedUpload(context.Context, Mixed) (web.NoContent, error)         { return web.NoContent{}, nil }
func DuplicateUpload(context.Context, Duplicate) (web.NoContent, error) { return web.NoContent{}, nil }
func ScalarUpload(context.Context, Scalar) (web.NoContent, error)       { return web.NoContent{}, nil }

type EmptyLocation struct {
	File web.File `form:"file" json:"-" query:""`
}
type EmptySchema struct {
	File web.File `form:"file" json:"-" schema:""`
}

func BadLocation(context.Context, EmptyLocation) (web.NoContent, error) { return web.NoContent{}, nil }
func BadSchema(context.Context, EmptySchema) (web.NoContent, error)     { return web.NoContent{}, nil }
func Register(r *web.Router) {
	web.Post(r, "/mixed", MixedUpload)
	web.Post(r, "/duplicate", DuplicateUpload)
	web.Post(r, "/scalar", ScalarUpload)
	web.Get(r, "/get", DuplicateUpload)
	web.Post(r, "/location", BadLocation)
	web.Post(r, "/schema", BadSchema)
}
