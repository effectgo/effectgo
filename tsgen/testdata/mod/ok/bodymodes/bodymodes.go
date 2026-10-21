package bodymodes

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

type Nested struct {
	Count int `json:"count" schema:"min=1"`
}
type Input struct {
	ID      int      `path:"id" json:"id"`
	Name    string   `json:"name" schema:"nonempty"`
	Nested  Nested   `json:"nested"`
	Names   []string `json:"names"`
	Enabled *bool    `json:"enabled"`
}
type Output struct {
	OK bool `json:"ok"`
}

func Legacy(_ context.Context, _ Input) (Output, error)     { return Output{}, nil }
func Empty(_ context.Context, _ Input) (Output, error)      { return Output{}, nil }
func BestEffort(_ context.Context, _ Input) (Output, error) { return Output{}, nil }
func Strict(_ context.Context, _ Input) (Output, error)     { return Output{}, nil }
func Remove(_ context.Context, _ Input) (Output, error)     { return Output{}, nil }

type FormInput struct {
	File        web.File  `form:"file" json:"-"`
	Attachment  *web.File `form:"attachment" json:"-"`
	CommitToken string    `form:"commit_token" json:"commit_token"`
	Names       []string  `form:"names" json:"names"`
	Enabled     *bool     `form:"enabled" json:"enabled"`
	Count       *int      `form:"count" json:"count"`
	Note        *string   `form:"note" json:"note"`
}

func Form(_ context.Context, _ FormInput) (Output, error) { return Output{}, nil }
func Register(r *web.Router) {
	web.Post(r, "/legacy/{id}", Legacy, web.JSONBody())
	web.Post(r, "/empty/{id}", Empty, web.JSONBodyEmpty())
	web.Post(r, "/best/{id}", BestEffort, web.JSONBodyBestEffort())
	web.Post(r, "/strict/{id}", Strict)
	web.Delete(r, "/remove/{id}", Remove)
	web.Post(r, "/form", Form, web.FormBody())
}
