package tsgen

import (
	"go/types"
	"os"
	"path/filepath"
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

func TestSourceFingerprintPortabilityAndBuildInputs(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("EFFECTGO_STUBS", "")
	root := t.TempDir()
	write := func(dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	module := "module example.com/portable\n\ngo 1.26.0\n"
	source := "package portable\ntype DTO struct { Name string }\n"
	write(root, "go.mod", module)
	write(root, "dto.go", source)
	inputs, err := collectSourceInputs(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(dir string) string {
		t.Helper()
		got, err := inputs.current(dir)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	baseline := inputs.Digest
	t.Setenv("GOFLAGS", "-p=1 -buildvcs=false -trimpath")
	if digest(root) != baseline {
		t.Fatal("nonsemantic build flags invalidated an identical contract")
	}
	t.Setenv("GOOS", "windows")
	t.Setenv("GOARCH", "amd64")
	if digest(root) != baseline {
		t.Fatal("host defaults invalidated platform-independent source")
	}
	relocated := t.TempDir()
	write(relocated, "go.mod", module)
	write(relocated, "dto.go", source)
	if digest(relocated) != baseline {
		t.Fatal("checkout location changed the fingerprint")
	}
	write(root, "extra_linux.go", "package portable\ntype LinuxOnly struct{}\n")
	windows := digest(root)
	t.Setenv("GOOS", "linux")
	if digest(root) == windows {
		t.Fatal("platform-selected source changed without invalidating the fingerprint")
	}
	write(root, "tagged.go", "//go:build special\n\npackage portable\ntype Tagged struct{}\n")
	untagged := digest(root)
	t.Setenv("GOFLAGS", "-tags=special")
	if digest(root) == untagged {
		t.Fatal("build-tag changes did not invalidate the fingerprint")
	}
}
