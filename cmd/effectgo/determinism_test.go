package main

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/effectgo/effectgo/effects"
	"github.com/effectgo/effectgo/errset"
)

// TestOutputIndependentOfScheduling runs both analyzers over a module with
// cross-package facts, once per scheduling mode (sequential action graph, and
// parallel with GOMAXPROCS 1, 4 and 8), and requires the diagnostics, the
// effect sets and the error sets, including their witness chains, to be
// identical. The action graph runs analyzers in goroutines and passes facts
// between packages, so an ordering or sharing bug would show up here as a
// difference between modes.
func TestOutputIndependentOfScheduling(t *testing.T) {
	dir := "../../examples/layers"
	base := analyzeDump(t, dir, true, 1)
	if !strings.Contains(base, "effects") {
		t.Fatalf("no analyzer output to compare:\n%s", base)
	}
	cases := []struct {
		name       string
		sequential bool
		procs      int
	}{
		{"parallel-1", false, 1},
		{"parallel-4", false, 4},
		{"parallel-8", false, 8},
		{"parallel-4-again", false, 4},
		{"sequential-8", true, 8},
	}
	for _, c := range cases {
		if got := analyzeDump(t, dir, c.sequential, c.procs); got != base {
			t.Errorf("%s differs from sequential run:\n--- sequential\n%s\n--- %s\n%s", c.name, base, c.name, got)
		}
	}
}

// analyzeDump loads the module in dir, runs both analyzers under the given
// scheduling mode and returns a sorted, line-per-item rendering of everything
// they produced.
func analyzeDump(t *testing.T, dir string, sequential bool, procs int) string {
	t.Helper()
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))

	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.LoadAllSyntax | packages.NeedModule,
		Dir:   dir,
		Tests: true,
	}, "./...")
	if err != nil {
		t.Fatalf("load %s: %v", dir, err)
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{effects.Analyzer, errset.Analyzer}, pkgs,
		&checker.Options{Sequential: sequential})
	if err != nil {
		t.Fatalf("analyze %s: %v", dir, err)
	}

	var lines []string
	for _, act := range graph.Roots {
		name := act.Analyzer.Name + " " + act.Package.PkgPath
		if act.Err != nil {
			lines = append(lines, name+" error: "+act.Err.Error())
		}
		for _, d := range act.Diagnostics {
			pos := act.Package.Fset.Position(d.Pos)
			lines = append(lines, fmt.Sprintf("diag %s:%d:%d %s", pos.Filename, pos.Line, pos.Column, d.Message))
		}
		switch res := act.Result.(type) {
		case effects.Result:
			for fn, effs := range res {
				lines = append(lines, fmt.Sprintf("effects %s: %v", fn.FullName(), effs))
			}
		case errset.Result:
			for fn, fe := range res.Funcs {
				lines = append(lines, fmt.Sprintf("errors %s: handler=%v %s", fn.FullName(), fe.Handler, renderVariants(fe.Variants)))
			}
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// renderVariants includes the witness chains, which are the part of a result
// most sensitive to evaluation order.
func renderVariants(vs []errset.Variant) string {
	var parts []string
	for _, v := range vs {
		parts = append(parts, fmt.Sprintf("%s via %s", v.Name, strings.Join(v.Chain, " -> ")))
	}
	return "[" + strings.Join(parts, " | ") + "]"
}
