package external

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

// Nonmigrated Huma routes coexist in a package but are outside effectgo ts.
// Their closures and pointer-input signatures must not stop rt/web generation.
func RegisterHuma(api huma.API) {
	huma.Get(api, "/api/legacy", func(context.Context, *struct{}) (*struct{ Body Result }, error) {
		return &struct{ Body Result }{Body: Result{Name: "legacy"}}, nil
	})
}
