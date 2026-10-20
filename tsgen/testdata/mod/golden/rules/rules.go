// Package rules is a golden case: every schema rule on the request side, with
// the check or the comment it gets, and a response type whose rules give no
// output.
package rules

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

// RulesIn is the request body of POST /api/rules.
type RulesIn struct {
	Age    int      `json:"age" schema:"min=18,max=130"`
	Count  uint     `json:"count" schema:"min=0"`
	Score  float64  `json:"score" schema:"min=0,max=1.5"`
	Ratio  *int     `json:"ratio,omitempty" schema:"min=1"`
	Tags   []string `json:"tags" schema:"min=1,max=5"`
	Labels []string `json:"labels" schema:"nonempty"`
	Name   string   `json:"name" schema:"trim,min=2,max=50"`
	Title  string   `json:"title" schema:"nonempty"`
	Notes  string   `json:"notes" schema:"singleline,max=2000"`
	Email  string   `json:"email" schema:"email"`
	Code   string   `json:"code" schema:"pattern=zip"`
}

// RulesOut is the response of POST /api/rules. Its rules are not sent to the client.
type RulesOut struct {
	Name string `json:"name" schema:"min=2,email"`
}

// Rules answers with the name.
func Rules(_ context.Context, in RulesIn) (RulesOut, error) {
	return RulesOut{Name: in.Name}, nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Post(r, "/api/rules", Rules)
}
