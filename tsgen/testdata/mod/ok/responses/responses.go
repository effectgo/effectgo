package responses

import (
	"context"
	"net/http"

	"github.com/effectgo/effectgo/rt/web"
)

type Output struct {
	Name string `json:"name"`
}

type Batch struct {
	ID      int      `json:"id"`
	Title   chan int `json:"title"` // dominated by the explicit output field
	Ignored chan int `json:"-"`
	hidden  chan int
}

type BatchExtra struct {
	Note     *string  `json:"note"`
	Labels   []string `json:"labels,omitempty"`
	Required []string `json:"required"`
}

type BatchDetail struct {
	Batch
	*BatchExtra
	Title string `json:"title"`
	Rows  []struct {
		SKU  string `json:"sku"`
		Meta struct {
			Active bool `json:"active"`
		} `json:"meta"`
	} `json:"rows"`
}

func Inherited(context.Context, web.NoInput) ([]BatchDetail, error) { return nil, nil }

type FileResult = web.FileResponse
type RedirectResult = web.RedirectResponse
type JSONResult = web.Response[Output]
type ConditionalInput struct {
	Range         *string `header:"Range" json:"range"`
	ModifiedSince *string `header:"If-Modified-Since" json:"modified_since"`
}

func Conditional(context.Context, ConditionalInput) (FileResult, error) { return FileResult{}, nil }

func Created(context.Context, web.NoInput) (Output, error)  { return Output{}, nil }
func Accepted(context.Context, web.NoInput) (Output, error) { return Output{}, nil }
func File(context.Context, web.NoInput) (FileResult, error) { return FileResult{}, nil }
func Stream(context.Context, web.NoInput) (web.StreamResponse, error) {
	return web.StreamResponse{}, nil
}
func Redirect(context.Context, web.NoInput) (RedirectResult, error) {
	return web.RedirectResponse{}, nil
}
func Empty(context.Context, web.NoInput) (web.NoContent, error) { return web.NoContent{}, nil }
func Dynamic(context.Context, web.NoInput) (JSONResult, error) {
	return web.Response[Output]{}, nil
}
func Register(r *web.Router) {
	web.Post(r, "/created", Created, web.SuccessStatus(http.StatusCreated))
	web.Post(r, "/accepted", Accepted, web.SuccessStatus(100+102))
	web.Get(r, "/file", File)
	web.Get(r, "/conditional", Conditional)
	web.Get(r, "/stream", Stream, web.SuccessStatusRange(200, 299))
	web.Get(r, "/redirect", Redirect, web.SuccessStatus(http.StatusFound))
	web.Delete(r, "/empty", Empty)
	web.Get(r, "/dynamic", Dynamic, web.SuccessStatuses(200, 201, 202, 204, 205))
	web.Get(r, "/inherited", Inherited)
}
