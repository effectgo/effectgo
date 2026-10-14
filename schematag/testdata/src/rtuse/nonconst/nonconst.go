// Package nonconst registers routes whose pattern is not a constant. The path
// checks need the pattern, so they are not made here: the path field and the
// wildcard below are silent. The checks that do not need the pattern still apply.
package nonconst

import (
	"context"
	"os"

	"github.com/faintghost/effectgo/rt/web"
)

var prefix = os.Getenv("API_PREFIX")

// Out is a valid output.
type Out struct{}

// In reads a path field whose name the pattern may not have.
type In struct {
	ID string `path:"id"`
}

// GetIn has a body field, which a GET cannot read.
type GetIn struct {
	Name string `json:"name"`
}

func handle(ctx context.Context, in In) (Out, error)       { return Out{}, nil }
func handleGet(ctx context.Context, in GetIn) (Out, error) { return Out{}, nil }

// Routes registers routes with a computed pattern.
func Routes(r *web.Router) {
	web.Get(r, prefix+"/{wild}", handle)
	web.Get(r, prefix+"/items", handleGet) // want `GET takes no body, but field Name`
}
