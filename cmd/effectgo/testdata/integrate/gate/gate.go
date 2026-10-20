// Package gate exercises CLI policies without changing the carried-status fixture.
package gate

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

var ErrDomain = errors.New("domain failure")

type Input struct{}
type Output struct{}

func Find(context.Context, *Input) (*Output, error)  { return nil, ErrDomain }
func Fresh(context.Context, *Input) (*Output, error) { return nil, errors.New("fresh") }
func Gin(c *gin.Context) {
	err := FindError()
	if err != nil {
		c.JSON(500, gin.H{"error": false})
	}
}
func FindError() error { return ErrDomain }

func Register(api huma.API, r *gin.Engine, path string) {
	huma.Get(api, "/domain", Find)
	huma.Get(api, "/fresh", Fresh)
	huma.Get(api, path, Find)
	r.GET("/domain", Gin)
	r.GET(path, Gin)
	// The current Gin adapter cannot resolve this variable registration.
	h := Gin
	r.GET("/variable", h)
}
