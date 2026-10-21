// Package b declares another Item.
package b

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

// Item is one thing of package b.
type Item struct {
	Name string `json:"name"`
}

// Get returns an Item.
func Get(ctx context.Context, in web.NoInput) (Item, error) { return Item{}, nil }
