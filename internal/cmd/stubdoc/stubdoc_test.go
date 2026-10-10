package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faintghost/effectgo/internal/stubs"
	shipped "github.com/faintghost/effectgo/stubs"
)

// TestPageIsCurrent fails when the committed page differs from a fresh render,
// so a change to a shipped stub file cannot leave the page stale.
func TestPageIsCurrent(t *testing.T) {
	want, err := render()
	if err != nil {
		t.Fatal(err)
	}
	// go test runs in the package directory, two levels below internal/cmd.
	got, err := os.ReadFile(filepath.Join("..", "..", "..", outPath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is out of date: run `go generate ./stubs` (or `go run ./internal/cmd/stubdoc`) and commit the result", outPath)
	}
}

// TestEveryRuleIsListed checks that the page has one table row per rule of
// every shipped file, so no rule can be dropped by the generator.
func TestEveryRuleIsListed(t *testing.T) {
	out, err := render()
	if err != nil {
		t.Fatal(err)
	}
	files, err := shipped.Files()
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "| `") {
			rows++
		}
	}
	total := 0
	for _, f := range files {
		rules, err := stubs.Parse(f.Name, f.Data)
		if err != nil {
			t.Fatal(err)
		}
		total += len(rules)
	}
	if rows != total {
		t.Fatalf("page lists %d rules, the shipped files have %d", rows, total)
	}
}
