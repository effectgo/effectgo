package main

import (
	"go/types"
	"regexp"
	"testing"

	"golang.org/x/tools/go/packages"
)

// diagnosticShape is the "path:line:col: message" form that a parser of the
// diagnostics matches. The summary line must not match it.
var diagnosticShape = regexp.MustCompile(`^\S+:\d+:\d+: `)

func pkg(id, forTest string) *packages.Package {
	return &packages.Package{ID: id, PkgPath: id, ForTest: forTest, Types: types.NewPackage(id, id)}
}

func TestSummaryLine(t *testing.T) {
	plain := []*packages.Package{pkg("a", ""), pkg("b", ""), pkg("c", "")}
	withTests := append(slicesClone(plain),
		pkg("a [a.test]", "a"), pkg("a.test", ""), pkg("a_test [a.test]", "a"))
	cases := []struct {
		name     string
		pkgs     []*packages.Package
		findings int
		hidden   int
		tests    bool
		want     string
	}{
		{"clean, no tests", plain, 0, 0, false, "effectgo: 3 packages analysed, 0 findings"},
		{"clean, with tests; the test main is not counted", withTests, 0, 0, true,
			"effectgo: 3 packages analysed (2 test variants), 0 findings"},
		{"findings and hidden duplicates", withTests, 6, 3, true,
			"effectgo: 3 packages analysed (2 test variants), 6 findings, 3 duplicates hidden (-effects.all or -errset.all to show them)"},
		{"singular forms", plain[:1], 1, 1, false,
			"effectgo: 1 package analysed, 1 finding, 1 duplicate hidden (-effects.all or -errset.all to show them)"},
	}
	for _, c := range cases {
		got := summaryLine(c.pkgs, c.findings, c.hidden, c.tests)
		if got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
		if diagnosticShape.MatchString(got) {
			t.Errorf("%s: the summary %q matches the diagnostic shape", c.name, got)
		}
	}
}

func TestIsTestMain(t *testing.T) {
	if !isTestMain(pkg("a.test", "")) {
		t.Error("a.test is the generated test main")
	}
	if isTestMain(pkg("a [a.test]", "a")) || isTestMain(pkg("a", "")) {
		t.Error("a test variant or a plain package is not a test main")
	}
}

func slicesClone(p []*packages.Package) []*packages.Package {
	return append([]*packages.Package(nil), p...)
}
