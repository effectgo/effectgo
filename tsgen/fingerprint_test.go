package tsgen

import (
	"go/types"
	"testing"

	"github.com/effectgo/effectgo/internal/rtshape"
)

func TestFingerprintTracksAnalyzedContract(t *testing.T) {
	field := &rtshape.Field{Name: "Name", JSON: "name", Type: &rtshape.Type{Go: types.Typ[types.String], Cat: rtshape.CatString}}
	shape := &rtshape.Shape{Root: &rtshape.Type{Cat: rtshape.CatStruct, Fields: []*rtshape.Field{field}}, JSONOutput: true}
	first := Result{Routes: []Route{{Method: "GET", Pattern: "/items", Out: shape, Pos: "old.go:1"}}}
	second := Result{Routes: []Route{{Method: "DELETE", Pattern: "/items/{id}", NoContent: true}}}
	before := Fingerprint([]Result{first, second})
	first.Routes[0].Pos = "new.go:100"
	if got := Fingerprint([]Result{second, first}); got != before {
		t.Fatal("positions or package traversal order changed the contract hash")
	}
	field.JSON = "displayName"
	if Fingerprint([]Result{first, second}) == before {
		t.Fatal("DTO JSON name change did not change the contract hash")
	}
	field.JSON = "name"
	field.Type.Go = types.Typ[types.Int32]
	field.Type.Cat = rtshape.CatInt
	int32Hash := Fingerprint([]Result{first, second})
	field.Type.Go = types.Typ[types.Uint32]
	if Fingerprint([]Result{first, second}) == int32Hash {
		t.Fatal("integer kind change did not change the contract hash")
	}
	field.Type.Go, field.Type.Cat = types.Typ[types.String], rtshape.CatString
	first.Routes[0].Pattern = "/renamed"
	if Fingerprint([]Result{first, second}) == before {
		t.Fatal("route change did not change the contract hash")
	}
}
