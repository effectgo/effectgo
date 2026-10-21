// Package nested is a golden case: a struct used twice in the request (emitted
// once), a struct used twice in the response (emitted once), and a nil slice of
// structs in the response.
package nested

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

// Address is used twice by OrderIn, the request.
type Address struct {
	Street string `json:"street"`
	City   string `json:"city"`
}

// OrderIn is the request body of POST /api/ship.
type OrderIn struct {
	Billing  Address  `json:"billing"`
	Shipping Address  `json:"shipping"`
	Tags     []string `json:"tags"`
}

// Place is used twice by Receipt, the response.
type Place struct {
	Label string  `json:"label"`
	Lat   float64 `json:"lat"`
}

// Line is an element of Receipt.Lines.
type Line struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

// Receipt is the response of POST /api/ship.
type Receipt struct {
	Origin Place  `json:"origin"`
	Dest   Place  `json:"dest"`
	Lines  []Line `json:"lines"`
}

// Ship answers with a receipt from the billing city.
func Ship(_ context.Context, in OrderIn) (Receipt, error) {
	return Receipt{Origin: Place{Label: in.Billing.City}}, nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Post(r, "/api/ship", Ship)
}
