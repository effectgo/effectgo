package effects

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// TestFuncContractsPackageDefaultSkipsTests: a package default applies to the
// functions of the package's files, not to those of a _test.go file. A test
// function's own directive still applies there.
func TestFuncContractsPackageDefaultSkipsTests(t *testing.T) {
	fset := token.NewFileSet()
	pkgCon := &contract{text: "//effectgo:package pure", allowed: map[string]bool{}}
	c := &checker{pass: &analysis.Pass{Fset: fset, Report: func(analysis.Diagnostic) {}}}

	parse := func(name, src string) *ast.File {
		f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	lib := parse("p.go", "package p\n\nfunc Plain() {}\n\n//effectgo:allow net\nfunc Own() {}\n")
	tests := parse("p_test.go", "package p\n\nfunc TestTable() {}\n\n//effectgo:pure\nfunc TestOwn() {}\n")

	cases := []struct {
		file *ast.File
		name string
		want string // "" for no contract, else the contract text
	}{
		{lib, "Plain", pkgCon.text},
		{lib, "Own", "//effectgo:allow net"},
		{tests, "TestTable", ""},
		{tests, "TestOwn", "//effectgo:pure"},
	}
	for _, tc := range cases {
		var fd *ast.FuncDecl
		for _, d := range tc.file.Decls {
			if f, ok := d.(*ast.FuncDecl); ok && f.Name.Name == tc.name {
				fd = f
			}
		}
		if fd == nil {
			t.Fatalf("no function %s", tc.name)
		}
		cons := c.funcContracts(fd, pkgCon)
		got := ""
		if len(cons) > 0 {
			got = cons[0].text
		}
		if got != tc.want {
			t.Errorf("%s: contract %q, want %q", tc.name, got, tc.want)
		}
	}
}
