// Package conflict registers the two Get handlers, whose Item types share a name.
package conflict

import (
	"github.com/faintghost/effectgo/rt/web"

	"example.com/tsbad/bad/conflict/a"
	"example.com/tsbad/bad/conflict/b"
)

// Routes registers both.
func Routes(r *web.Router) {
	web.Get(r, "/a", a.Get)
	web.Get(r, "/b", b.Get)
}
