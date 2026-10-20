package main

import (
	"flag"
	"fmt"
	"go/types"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/faintghost/effectgo/directive"
	"github.com/faintghost/effectgo/effects"
	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/exhaustive"
	"github.com/faintghost/effectgo/internal/hidden"
	"github.com/faintghost/effectgo/mustuse"
	"github.com/faintghost/effectgo/schematag"
)

// The standalone run, effectgo ./... It is not the multichecker: that driver
// prints its diagnostics and exits, so it cannot print the summary line that
// follows them. This run uses the checker API that the subcommands use, prints
// the diagnostics the way the multichecker does (text on stderr, or -json on
// stdout), and ends with the summary line on stderr. The go vet invocations
// (-flags, -V, a .cfg file) and -help still go through the multichecker.

// standaloneAnalyzers are the analyzers of the standalone run. signature runs
// only under effectgo sig.
var standaloneAnalyzers = []*analysis.Analyzer{effects.Analyzer, errset.Analyzer, mustuse.Analyzer, exhaustive.Analyzer, schematag.Analyzer, directive.Analyzer}

var (
	includeTests bool
	jsonOut      bool
	contextLines int
)

// registerStandaloneFlags adds the driver's flags and the analyzers' flags to
// the global flag set, named as the multichecker names them (-test, -json, -c,
// and analyzer.flag for each analyzer flag).
func registerStandaloneFlags() {
	flag.BoolVar(&includeTests, "test", true, "analyze the test files too, as go vet does; -test=false skips them")
	flag.BoolVar(&jsonOut, "json", false, "print the diagnostics as JSON on stdout (the driver's format, not the HTTP schema)")
	flag.IntVar(&contextLines, "c", -1, "print this many lines of source context around each diagnostic")
	bindAnalyzerFlags(flag.CommandLine)
}

// bindAnalyzerFlags shares the analyzers' native flags across CLI drivers.
func bindAnalyzerFlags(fs *flag.FlagSet) {
	for _, a := range standaloneAnalyzers {
		a.Flags.VisitAll(func(f *flag.Flag) {
			fs.Var(f.Value, a.Name+"."+f.Name, f.Usage)
		})
	}
}

// runStandalone runs the standalone analysis over the patterns in args (os.Args)
// and returns the exit code: 0 for no findings, 1 for a load or analysis error,
// 3 when diagnostics were reported (see docs/reference/cli.md).
func runStandalone(args []string) int {
	registerStandaloneFlags()
	if err := flag.CommandLine.Parse(args[1:]); err != nil {
		return 2
	}
	if err := checkFlagOrder("", flag.CommandLine, args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	// After the flag check: a flag written after the patterns would otherwise
	// be read as a pattern, and its value as a stub or a directory.
	if err := checkStubs(args); err != nil {
		fmt.Fprintf(os.Stderr, "effectgo: %v\n", err)
		return 1
	}
	patterns := flag.Args()
	if len(patterns) == 0 {
		fmt.Fprintln(os.Stderr, "usage: effectgo [flags] packages   (see effectgo -help)")
		return 1
	}
	cfg := &packages.Config{Mode: packages.LoadAllSyntax | packages.NeedModule | packages.NeedForTest, Tests: includeTests}
	initial, err := packages.Load(cfg, patterns...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "effectgo: %v\n", err)
		return 1
	}
	if len(initial) == 0 {
		fmt.Fprintf(os.Stderr, "effectgo: %v matched no packages\n", patterns)
		return 1
	}
	exit := 0
	if n := packages.PrintErrors(initial); n > 0 {
		exit = 1 // reported, but the analysis still runs, as in the multichecker
	}
	graph, err := checker.Analyze(standaloneAnalyzers, analysed(initial), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var printErr error
	if jsonOut {
		printErr = graph.PrintJSON(os.Stdout)
	} else {
		printErr = graph.PrintText(os.Stderr, contextLines)
	}
	if printErr != nil {
		return 1
	}

	numErrors, findings := countFindings(graph)
	var pkgTypes []*types.Package
	for _, p := range analysed(initial) {
		pkgTypes = append(pkgTypes, p.Types)
	}
	nHidden := hidden.Total("effects", pkgTypes) + hidden.Total("errset", pkgTypes)
	fmt.Fprintln(os.Stderr, summaryLine(initial, findings, nHidden, includeTests))

	switch {
	case numErrors > 0:
		return 1
	case !jsonOut && findings > 0:
		return 3
	}
	return exit
}

// countFindings returns the number of root actions that failed and the number
// of distinct diagnostics of the roots. Distinct means what the printer shows:
// the printer drops a diagnostic that has the same position, analyzer and text
// as an earlier one (a file that belongs to a package and to its test variant).
func countFindings(graph *checker.Graph) (numErrors, findings int) {
	type key struct {
		pos, end, analyzer, msg string
	}
	seen := map[key]bool{}
	for act := range graph.All() {
		switch {
		case act.Err != nil:
			numErrors++
		case act.IsRoot:
			for _, d := range act.Diagnostics {
				k := key{
					act.Package.Fset.Position(d.Pos).String(),
					act.Package.Fset.Position(d.End).String(),
					act.Analyzer.Name, d.Message,
				}
				if !seen[k] {
					seen[k] = true
					findings++
				}
			}
		}
	}
	return numErrors, findings
}

// analysed returns the packages that the run analyzes as roots: the packages
// of the patterns and their test variants, without the generated test main
// packages (import path "<pkg>.test"), which contain no source of their own.
func analysed(initial []*packages.Package) []*packages.Package {
	var out []*packages.Package
	for _, p := range initial {
		if isTestMain(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// isTestMain reports whether p is a generated test main package.
func isTestMain(p *packages.Package) bool {
	return strings.HasSuffix(p.ID, ".test") && p.ForTest == ""
}

// summaryLine is the last line of a standalone run, for example
// "effectgo: 6 packages analysed (2 test variants), 0 findings". It starts with
// "effectgo: " and has no "path:line:col:" prefix, so a parser of the
// diagnostics does not take it for one. The duplicates clause appears only when
// some violations were hidden (see docs/reference/analyzers.md).
func summaryLine(initial []*packages.Package, findings, nHidden int, tests bool) string {
	var pkgs, variants int
	for _, p := range analysed(initial) {
		if p.ForTest != "" {
			variants++
		} else {
			pkgs++
		}
	}
	s := fmt.Sprintf("effectgo: %d %s analysed", pkgs, plural(pkgs, "package", "packages"))
	if tests {
		s += fmt.Sprintf(" (%d test %s)", variants, plural(variants, "variant", "variants"))
	}
	s += fmt.Sprintf(", %d %s", findings, plural(findings, "finding", "findings"))
	if nHidden > 0 {
		s += fmt.Sprintf(", %d %s hidden (-effects.all or -errset.all to show them)", nHidden, plural(nHidden, "duplicate", "duplicates"))
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
