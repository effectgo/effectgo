// Package problem has a field type that rt/schema refuses: a channel.
package problem

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

// In has a channel field.
type In struct {
	Name string   `json:"name"`
	Ch   chan int `json:"ch"`
}

// Out is the response.
type Out struct {
	OK bool `json:"ok"`
}

// Save takes In.
func Save(ctx context.Context, in In) (Out, error) { return Out{OK: true}, nil }

// Routes registers Save.
func Routes(r *web.Router) { web.Post(r, "/p", Save) }
