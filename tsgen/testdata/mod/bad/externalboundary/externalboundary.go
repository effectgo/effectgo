package main

import (
	"context"
	"fmt"
	"os"

	"github.com/effectgo/effectgo/rt/schema"
	"github.com/effectgo/effectgo/rt/web"
)

type Detail struct {
	Message string `json:"message"`
}

type EmbeddedError struct {
	Detail
}

func (EmbeddedError) Error() string   { return "embedded error" }
func (EmbeddedError) StatusCode() int { return 401 }

type InvalidRules struct {
	Detail string `json:"detail" schema:"not-a-rule"`
}

func (InvalidRules) Error() string   { return "invalid rules" }
func (InvalidRules) StatusCode() int { return 401 }

func Read(context.Context, web.NoInput) (web.NoContent, error) {
	return web.NoContent{}, nil
}

func RegisterEmbedded(r *web.Router) {
	web.Get(r, "/embedded", Read, web.ExternalError[EmbeddedError]())
}

func RegisterRules(r *web.Router) {
	web.Get(r, "/rules", Read, web.ExternalError[InvalidRules]())
}

func main() {
	defer func() {
		if failure := recover(); failure != nil {
			fmt.Print(failure)
		}
	}()
	switch os.Args[1] {
	case "embedded":
		RegisterEmbedded(web.New())
	case "rules":
		RegisterRules(web.New())
	case "input":
		fmt.Print(schema.Compile[EmbeddedError]())
		return
	}
	fmt.Print("accepted")
}
