// Package a declares an Item.
package a

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

// Item is one thing of package a.
type Item struct {
	ID int64 `json:"id"`
}

// Get returns an Item.
func Get(ctx context.Context, in web.NoInput) (Item, error) { return Item{}, nil }
