// Package repeat is a case for the error list: the handler returns web.InternalError
// itself, so the framework's InternalError is already listed, and must not be
// listed twice.
package repeat

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

// In is the query of GET /api/repeat.
type In struct {
	Q string `query:"q"`
}

// Get returns the framework's own internal error.
func Get(_ context.Context, in In) (web.NoContent, error) {
	if in.Q == "" {
		return web.NoContent{}, web.InternalError{}
	}
	return web.NoContent{}, nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Get(r, "/api/repeat", Get)
}
