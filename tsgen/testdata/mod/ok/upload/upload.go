package upload

import (
	"context"
	"github.com/effectgo/effectgo/rt/web"
)

type Input struct {
	ID         int       `path:"id" json:"id"`
	File       web.File  `form:"file" json:"-"`
	Attachment *web.File `form:"attachment" json:"-"`
}
type Output struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

func Upload(_ context.Context, in Input) (Output, error) {
	return Output{Name: in.File.Filename, Size: len(in.File.Data)}, nil
}

type JSONInput struct {
	Item Input `json:"item"`
}

func JSON(_ context.Context, in JSONInput) (Output, error) {
	return Output{Name: "json", Size: in.Item.ID}, nil
}
func Register(r *web.Router) {
	web.Post(r, "/upload/{id}", Upload)
	web.Post(r, "/json", JSON)
}
