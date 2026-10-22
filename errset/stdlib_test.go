package errset

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

func TestStandardLibraryDoesNotExportBodyDerivedErrors(t *testing.T) {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax}, "errors")
	if err != nil || packages.PrintErrors(pkgs) != 0 {
		t.Fatalf("load errors: %v", err)
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{Analyzer}, pkgs, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range graph.Roots {
		if action.Err != nil {
			t.Fatal(action.Err)
		}
		result := action.Result.(Result)
		if len(result.Funcs) != 0 || len(result.NoErrors) != 0 || len(action.Diagnostics) != 0 {
			t.Fatalf("standard-library bodies leaked into the error contract: package=%s funcs=%d noErrors=%d diagnostics=%v", action.Package.PkgPath, len(result.Funcs), len(result.NoErrors), action.Diagnostics)
		}
	}
}
