package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractFingerprintCLI(t *testing.T) {
	bin := buildEffectgo(t)
	dir := t.TempDir()
	runtimeDir, err := filepath.Abs("../../rt")
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", fmt.Sprintf("module example.com/fingerprint\n\ngo 1.26.0\n\nrequire github.com/effectgo/effectgo/rt v0.0.0\nreplace github.com/effectgo/effectgo/rt => %s\n", filepath.ToSlash(runtimeDir)))
	model := "package dto\ntype Item struct { Name string `json:\"name\"` }\n"
	write("dto/model.go", model)
	routes := `package app
import (
 "context"
 "example.com/fingerprint/dto"
 "github.com/effectgo/effectgo/rt/web"
)
func Item(context.Context, web.NoInput) (dto.Item, error) { return dto.Item{}, nil }
func Register(r *web.Router) { web.Get(r, "/item", Item) }
`
	write("routes.go", routes)
	generate := func() string {
		t.Helper()
		_, stderr, code := runIn(t, bin, dir, "ts", "-o", "api.gen.ts", "./...")
		if code != 0 {
			t.Fatalf("generate: exit %d: %s", code, stderr)
		}
		body, err := os.ReadFile(filepath.Join(dir, "api.gen.ts"))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	check := func(want int, files ...string) {
		t.Helper()
		args := append([]string{"contract", "fingerprint", "--check"}, files...)
		stdout, stderr, code := runIn(t, bin, dir, args...)
		if code != want || stdout != "" || want == 1 && !strings.Contains(stderr, "stale contract fingerprint") {
			t.Fatalf("check: exit %d, want %d; stdout=%s stderr=%s", code, want, stdout, stderr)
		}
	}
	original := generate()
	if !strings.Contains(original, "// contract-fingerprint: sha256:") || !strings.Contains(original, "// effectgo-inputs:") {
		t.Fatal("generation omitted the analyzed hash or source inputs")
	}
	check(0) // discovery
	check(0, "api.gen.ts")

	write("new.go", "package app\nfunc Unrelated() int { return 1 }\n")
	check(0)
	write("new.go", "package app\nfunc Unrelated() int { return 2 }\n")
	check(0)
	write("routes.go", strings.Replace(routes, "return dto.Item{}, nil", "return dto.Item{Name: \"changed\"}, nil", 1))
	check(0)
	write("routes.go", "// Formatting and comments do not change the contract.\n"+strings.ReplaceAll(routes, "web.Get(r,", "web.Get( r,"))
	check(0)
	write("routes.go", routes)
	write("dto/model.go", "package dto\n// DTO documentation.\ntype Item struct{ Name string `json:\"name\"` }\n")
	check(0)
	write("dto/model.go", model)
	write("routes.go", strings.Replace(routes, "func Register", "func Other(context.Context, web.NoInput) (dto.Item, error) { return dto.Item{}, nil }\nfunc Register", 1))
	check(0)
	write("routes.go", strings.Replace(strings.Replace(routes, "func Register", "func Other(context.Context, web.NoInput) (dto.Item, error) { return dto.Item{}, nil }\nfunc Register", 1), "\"/item\", Item", "\"/item\", Other", 1))
	check(1)
	write("routes.go", routes)
	for _, changed := range []string{
		strings.Replace(model, "Name string", "Label string", 1),
		strings.Replace(model, "Name string", "Name int", 1),
	} {
		write("dto/model.go", changed)
		check(1)
		generate()
		check(0)
		write("dto/model.go", model)
		generate()
	}
	original = generate()

	// Invalid source cannot establish that the analyzed contract is current.
	write("dto/model.go", "package dto\nthis is not valid Go\n")
	check(1)
	body, _ := os.ReadFile(filepath.Join(dir, "api.gen.ts"))
	if string(body) != original {
		t.Fatal("freshness check rewrote the generated client")
	}
	write("dto/model.go", strings.Replace(model, "Name string", "Added bool `json:\"added\"`; Name string", 1))
	check(1)
	regenerated := generate()
	if regenerated == original || !strings.Contains(regenerated, "added: Schema.Boolean") {
		t.Fatal("regeneration did not include the imported DTO field")
	}
	check(0)

	write("routes.go", strings.Replace(routes, "/item", "/renamed", 1))
	check(1)
	write("routes.go", routes)
	check(0)
	write("new.go", "package app\nconst Added = true\n")
	check(0)
	if err := os.Remove(filepath.Join(dir, "new.go")); err != nil {
		t.Fatal(err)
	}
	check(0)
	if err := os.Remove(filepath.Join(dir, "dto/model.go")); err != nil {
		t.Fatal(err)
	}
	check(1)
}
