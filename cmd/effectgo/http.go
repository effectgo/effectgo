package main

// The http subcommand: the gin and the Huma HTTP adapters over ONE load of the
// packages, printed in the shared schema
// {severity, framework, route, handler, pos, variant, status, via, reason}.
// Both adapters keep their own subcommands (http-gin, http-huma) unchanged; this
// one is the union of their findings, sorted by severity, then position.
//
//	effectgo http [-json] [-all] [-stubs=...] [packages]

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	ginhttp "github.com/faintghost/effectgo/adapters/gin"
	huma "github.com/faintghost/effectgo/adapters/huma"
	"github.com/faintghost/effectgo/internal/stubs"
)

// severityOrder ranks the severities of the shared schema. The findings print
// high first, then medium, leak and info; an unknown severity comes last.
var severityOrder = map[string]int{"high": 0, "medium": 1, "leak": 2, "info": 3}

// httpSummary counts what each adapter saw. The info counts include the info
// findings that are not printed (without -all).
type httpSummary struct {
	ginHandlers, ginRoutes    int
	humaRoutes, humaResolved  int
	gin, huma                 map[string]int // findings per severity
	ginCoverage, humaCoverage httpCoverage
}

// runHTTP implements `effectgo http [-json] [-all] [-stubs ...] [packages]`. It
// returns 0 on success, 2 on a usage error and 1 when the run fails.
func runHTTP(args []string) int {
	fs := flag.NewFlagSet("effectgo http", flag.ContinueOnError)
	bindEngine(fs)
	fs.StringVar(&stubs.Flag, "stubs", stubs.Flag,
		"comma-separated stub files; falls back to $"+stubs.Env+" when empty")
	jsonOut := fs.Bool("json", false, "one JSON object per line (the shared HTTP schema)")
	all := fs.Bool("all", false, "also print the info findings (opaque ?, fresh new@ and foreign-kind variants)")
	opts := bindHTTPOptions(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := checkFlagOrder("http", fs, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	out, sum, err := collectHTTP(patterns, *all)
	if err != nil {
		fmt.Fprintf(os.Stderr, "effectgo http: %v\n", err)
		return 1
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		for _, hf := range out {
			if err := enc.Encode(hf); err != nil {
				fmt.Fprintf(os.Stderr, "effectgo http: %v\n", err)
				return 1
			}
		}
	} else {
		printHTTPText(os.Stdout, out)
	}
	if opts.Coverage == "json" {
		if err := writeCoverageJSON(os.Stderr, httpUnionCoverage(sum)); err != nil {
			fmt.Fprintf(os.Stderr, "effectgo http: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(os.Stderr, "effectgo http: gin handlers=%d routes=%d high=%d medium=%d leak=%d info=%d; huma routes=%d resolved=%d high=%d medium=%d info=%d\n",
			sum.ginHandlers, sum.ginRoutes, sum.gin["high"], sum.gin["medium"], sum.gin["leak"], sum.gin["info"],
			sum.humaRoutes, sum.humaResolved, sum.huma["high"], sum.huma["medium"], sum.huma["info"])
	}
	counts := map[string]int{}
	for severity, count := range sum.gin {
		counts[severity] += count
	}
	for severity, count := range sum.huma {
		counts[severity] += count
	}
	return gateHTTP(opts, counts, sum.ginCoverage.Unresolved+sum.humaCoverage.Unresolved)
}

// collectHTTP loads the packages once, runs both adapters on that load, and
// returns the printed findings sorted by severity, then position (route, then
// variant, break ties). Info findings are included only when all is set.
func collectHTTP(patterns []string, all bool) ([]httpFinding, httpSummary, error) {
	cwd := mustGetwd()
	graph, err := analyzeHTTP(patterns, ginhttp.Analyzer, huma.Analyzer)
	if err != nil {
		return nil, httpSummary{}, err
	}
	gFindings, gRun, err := ginFromGraph(graph)
	if err != nil {
		return nil, httpSummary{}, err
	}
	hResults, err := humaFromGraph(graph, cwd)
	if err != nil {
		return nil, httpSummary{}, err
	}
	hRep := huma.Classify(hResults)

	sum := httpSummary{
		ginHandlers:  gRun.Handlers,
		ginRoutes:    len(gRun.Routes),
		humaRoutes:   hRep.Stats.Routes,
		humaResolved: hRep.Stats.Resolved,
		gin:          map[string]int{},
		huma:         map[string]int{},
	}
	var out []httpFinding
	for _, f := range gFindings {
		hf := classify(f, gRun)
		sum.gin[hf.Severity]++
		if hf.Severity == "info" && !all {
			continue
		}
		out = append(out, relativize(hf, cwd))
	}
	for _, f := range hRep.Findings {
		sum.huma[f.Severity]++
		if f.Severity == "info" && !all {
			continue
		}
		// huma.Finding has the same fields, in the same order, as httpFinding.
		out = append(out, httpFinding(f))
	}
	sum.ginCoverage = ginCoverage(gRun, sum.gin)
	sum.humaCoverage = humaCoverage(hRep)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := rankOf(a.Severity), rankOf(b.Severity); ra != rb {
			return ra < rb
		}
		if a.Pos != b.Pos {
			return a.Pos < b.Pos
		}
		if a.Route != b.Route {
			return a.Route < b.Route
		}
		return a.Variant < b.Variant
	})
	return out, sum, nil
}

// rankOf is the position of a severity in severityOrder; an unknown one ranks last.
func rankOf(sev string) int {
	if r, ok := severityOrder[sev]; ok {
		return r
	}
	return len(severityOrder)
}

// analyzeHTTP loads the packages once and runs the analyzers on them. It fails
// when the packages do not load or an analyzer fails on a package. The
// analyzers share one load, so a package is parsed and type-checked once.
func analyzeHTTP(patterns []string, analyzers ...*analysis.Analyzer) (*checker.Graph, error) {
	// NeedModule gives the analysis passes their module, which errset uses to
	// tell project errors from foreign ones. Tests are off, as in standalone runs.
	cfg := &packages.Config{Mode: packages.LoadAllSyntax | packages.NeedModule}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("the packages do not load")
	}
	graph, err := checker.Analyze(analyzers, pkgs, nil)
	if err != nil {
		return nil, err
	}
	for _, act := range graph.Roots {
		if act.Err != nil {
			return nil, fmt.Errorf("%s: %v", act.Package, act.Err)
		}
	}
	return graph, nil
}
