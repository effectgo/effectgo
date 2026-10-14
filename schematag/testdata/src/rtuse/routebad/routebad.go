// Package routebad registers routes whose inputs do not match their pattern or
// method: path and query tags, bodies on GET and DELETE, a NoInput with a
// wildcard and a non-struct input. Route-level problems are reported at the
// route call; a field problem of an output type is reported at its field.
package routebad

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

// Inner is a struct, so it is not a scalar query value.
type Inner struct{ X int }

// Out is a valid output.
type Out struct {
	Name string `json:"name"`
}

// BadOut has an output type that schema cannot encode.
type BadOut struct {
	C chan int `json:"c"` // want `field BadOut.C: type chan int is not supported`
}

type pathMismatch struct {
	ID    int    `path:"id"`
	Other string `path:"other"`
}

type notScalar struct {
	Ids []int `path:"ids"`
}

type bodyOnGet struct {
	Name string `json:"name"`
	ID   int    `path:"id"`
}

type bodyOnDelete struct {
	Name string `json:"name"`
}

type queryStruct struct {
	Q Inner `query:"q"`
}

type bothTags struct {
	X string `path:"x" query:"x"`
}

type dashPath struct {
	X string `json:"-" path:"x"`
}

type emptyQuery struct {
	X string `query:""`
}

type wildcardNoField struct {
	Name string `query:"name"`
}

func handlePath(ctx context.Context, in pathMismatch) (Out, error)       { return Out{}, nil }
func handleNotScalar(ctx context.Context, in notScalar) (Out, error)     { return Out{}, nil }
func handleBodyGet(ctx context.Context, in bodyOnGet) (Out, error)       { return Out{}, nil }
func handleBodyDelete(ctx context.Context, in bodyOnDelete) (Out, error) { return Out{}, nil }
func handleQueryStruct(ctx context.Context, in queryStruct) (Out, error) { return Out{}, nil }
func handleBoth(ctx context.Context, in bothTags) (Out, error)           { return Out{}, nil }
func handleDash(ctx context.Context, in dashPath) (Out, error)           { return Out{}, nil }
func handleEmptyQuery(ctx context.Context, in emptyQuery) (Out, error)   { return Out{}, nil }
func handleScalar(ctx context.Context, in string) (Out, error)           { return Out{}, nil }
func handleNoInput(ctx context.Context, in web.NoInput) (Out, error)     { return Out{}, nil }
func handleBadOut(ctx context.Context, in wildcardNoField) (BadOut, error) {
	return BadOut{}, nil
}

// Routes registers the bad routes.
func Routes(r *web.Router) {
	web.Get(r, "/items/{other}", handlePath)       // want `GET /items/\{other\}: field ID has path:"id", but the pattern has no \{id\}`
	web.Get(r, "/list/{ids}", handleNotScalar)     // want `path field Ids must be a scalar`
	web.Get(r, "/body/{id}", handleBodyGet)        // want `GET takes no body, but field Name`
	web.Delete(r, "/body", handleBodyDelete)       // want `DELETE takes no body, but field Name`
	web.Get(r, "/q", handleQueryStruct)            // want `query field Q must be a scalar or a slice of scalars, got Inner`
	web.Get(r, "/both", handleBoth)                // want `has both path and query tags`
	web.Get(r, "/dash", handleDash)                // want `has a path or query tag but json:"-"`
	web.Get(r, "/eq", handleEmptyQuery)            // want `has an empty query name`
	web.Get(r, "/scalar", handleScalar)            // want `input string must be a struct or web.NoInput`
	web.Get(r, "/none/{id}", handleNoInput)        // want `pattern has \{id\}, but the input is web.NoInput`
	web.Get(r, "/nofield/{key}", handleEmptyQuery) // want `pattern \{key\} has no path field` `has an empty query name`
	web.Get(r, "/out", handleBadOut)
}
