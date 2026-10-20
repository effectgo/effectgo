// Package app is a golden case: it registers the methods of package catalog, so
// the handlers and their error sets come from another package.
package app

import (
	"example.com/tsbad/golden/catalog"
	"github.com/faintghost/effectgo/rt/web"
)

// Routes registers the catalog routes.
func Routes(r *web.Router, c *catalog.Catalog) {
	web.Get(r, "/api/items/{id}", c.Get)
	web.Delete(r, "/api/items/{id}", c.Remove)
}
