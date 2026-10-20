// Package cross brings a type of another package into rt. The problems of that
// type are reported at the route that reads it, with the field's position.
package cross

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
	"rtmodels"
)

// Out is a valid output.
type Out struct{}

func handle(ctx context.Context, in rtmodels.Contact) (Out, error) { return Out{}, nil }

// Routes registers the route.
func Routes(r *web.Router) {
	web.Post(r, "/c", handle) // want `route POST /c: input rtmodels.Contact: field Contact.Name: schema tag "trim,nonempty,bogus": unknown rule "bogus"` `route POST /c: input rtmodels.Contact: field Contact.Status: type rtmodels.Status is a sealed enum`
}
