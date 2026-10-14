package main

// The http-gin subcommand. It runs the gin adapter (adapters/gin) through the
// checker API, like the openapi subcommand, and then classifies the findings
// against the explicit mappings of the whole run: a named variant that is
// mapped to a non-5xx status anywhere is an inconsistent mapping when it
// reaches a 5xx response elsewhere (high).

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis/checker"

	ginhttp "github.com/faintghost/effectgo/adapters/gin"
	"github.com/faintghost/effectgo/internal/stubs"
)

// httpFinding is one output record, in the shared schema of the HTTP checks.
type httpFinding struct {
	Severity  string `json:"severity"`
	Framework string `json:"framework"`
	Route     string `json:"route"`
	Handler   string `json:"handler"`
	Pos       string `json:"pos"`
	Variant   string `json:"variant"`
	Status    int    `json:"status"`
	Via       string `json:"via"`
	Reason    string `json:"reason"`
}

// runHTTPGin implements `effectgo http-gin [-json] [-all] [-mappers] [-stubs ...] [packages]`.
// It returns 0 on success, 2 on a usage error and 1 when the run fails.
func runHTTPGin(args []string) int {
	fs := flag.NewFlagSet("effectgo http-gin", flag.ContinueOnError)
	bindEngine(fs)
	fs.StringVar(&stubs.Flag, "stubs", stubs.Flag,
		"comma-separated stub files; falls back to $"+stubs.Env+" when empty")
	jsonOut := fs.Bool("json", false, "one JSON object per line (the shared HTTP schema)")
	all := fs.Bool("all", false, "also print the info findings (opaque ? and fresh new@ errors)")
	mappers := fs.Bool("mappers", false, "list the detected sink and status mappers on stderr")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := checkFlagOrder("http-gin", fs, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	findings, res, err := collectGin(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "effectgo http-gin: %v\n", err)
		return 1
	}
	if *mappers {
		printMappers(os.Stderr, res)
	}
	cwd, _ := os.Getwd()
	counts := map[string]int{}
	var out []httpFinding
	for _, f := range findings {
		hf := classify(f, res)
		counts[hf.Severity]++
		if hf.Severity == "info" && !*all {
			continue
		}
		out = append(out, relativize(hf, cwd))
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		for _, hf := range out {
			if err := enc.Encode(hf); err != nil {
				fmt.Fprintf(os.Stderr, "effectgo http-gin: %v\n", err)
				return 1
			}
		}
	} else {
		printHTTPText(os.Stdout, out)
	}
	fmt.Fprintf(os.Stderr, "effectgo http-gin: handlers=%d mappers=%d routes=%d high=%d medium=%d info=%d leak=%d\n",
		res.Handlers, len(res.Mappers), len(res.Routes), counts["high"], counts["medium"], counts["info"], counts["leak"])
	return 0
}

// ginRun is the aggregate of the adapter's results over the packages.
type ginRun struct {
	Handlers int
	Mappers  []ginhttp.MapperInfo
	Routes   []ginhttp.Route
	// mapped is the set of variant keys that have a non-5xx explicit mapping.
	mapped map[string]bool
	// routeOf joins a handler position to its route, "METHOD path".
	routeOf map[string]string
}

// collectGin loads the packages, runs the adapter, and returns its findings
// (sorted by position) with the aggregate.
func collectGin(patterns []string) ([]ginhttp.Finding, *ginRun, error) {
	graph, err := analyzeHTTP(patterns, ginhttp.Analyzer)
	if err != nil {
		return nil, nil, err
	}
	return ginFromGraph(graph)
}

// ginFromGraph reads the gin results of an analyzed graph (see analyzeHTTP).
func ginFromGraph(graph *checker.Graph) ([]ginhttp.Finding, *ginRun, error) {
	run := &ginRun{mapped: map[string]bool{}, routeOf: map[string]string{}}
	var findings []ginhttp.Finding
	for _, act := range graph.Roots {
		if act.Err != nil {
			return nil, nil, fmt.Errorf("%s: %v", act.Package, act.Err)
		}
		r, ok := act.Result.(ginhttp.Result)
		if !ok {
			continue
		}
		findings = append(findings, r.Findings...)
		run.Handlers += r.Handlers
		run.Mappers = append(run.Mappers, r.Mappers...)
		run.Routes = append(run.Routes, r.Routes...)
		for _, o := range r.Obs {
			if o.Status > 0 && o.Status < 500 {
				run.mapped[o.Key] = true
			}
		}
	}
	for _, rt := range run.Routes {
		key := rt.HandlerPos
		if _, dup := run.routeOf[key]; !dup {
			path := rt.Path
			if path == "" {
				path = "(non-constant path)"
			}
			run.routeOf[key] = rt.Method + " " + path
		}
	}
	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Pos < findings[j].Pos })
	return findings, run, nil
}

// classify assigns the severity and the reason of one finding.
func classify(f ginhttp.Finding, run *ginRun) httpFinding {
	hf := httpFinding{
		Framework: "gin",
		Route:     run.routeOf[f.HandlerPos],
		Handler:   f.Handler,
		Pos:       f.Pos,
		Variant:   f.Variant,
		Status:    f.Status,
		Via:       f.Via,
	}
	if f.Kind == "leak" {
		hf.Severity = "leak"
		hf.Reason = leakReason(f.Status)
		return hf
	}
	named := f.ErrKind == "sentinel" || f.ErrKind == "type"
	switch {
	case !named:
		hf.Severity = "info"
		hf.Reason = fmt.Sprintf("%s (not a named variant) reaches a %d response", f.Variant, f.Status)
	case run.mapped[f.Key]:
		hf.Severity = "high"
		hf.Reason = fmt.Sprintf("%s is mapped to a non-5xx status elsewhere in the project, but reaches a %d response here", f.Variant, f.Status)
	default:
		hf.Severity = "medium"
		hf.Reason = fmt.Sprintf("%s reaches a %d response and no other mapping in the project covers it", f.Variant, f.Status)
	}
	if named && !f.Project {
		hf.Reason += " (foreign variant)"
	}
	return hf
}

// leakReason is the reason text of a leak finding, by status class.
func leakReason(st int) string {
	switch {
	case st >= 500:
		return "error text (err.Error() or an error formatted into a string) is written into a 5xx response body; internals can leak"
	case st >= 400:
		return "error text is written into a 4xx response body; validation messages can be intended, internal details cannot"
	}
	return "error text is written into a response body whose status is not a constant"
}

// relativize rewrites the absolute paths of a record to paths under the working directory.
func relativize(hf httpFinding, cwd string) httpFinding {
	trim := func(s string) string {
		if cwd == "" {
			return s
		}
		return strings.ReplaceAll(s, cwd+string(filepath.Separator), "")
	}
	hf.Handler = trim(hf.Handler)
	hf.Pos = trim(hf.Pos)
	hf.Via = trim(hf.Via)
	hf.Reason = trim(hf.Reason)
	return hf
}

// printHTTPText prints one line per finding, in the text form (http-gin and http).
func printHTTPText(w io.Writer, fs []httpFinding) {
	for _, f := range fs {
		route := f.Route
		if route == "" {
			route = "route unknown"
		}
		fmt.Fprintf(w, "%s: %s: %s -> %d (%s) in %s\n    via %s\n    %s\n",
			f.Pos, f.Severity, f.Variant, f.Status, route, f.Handler, f.Via, f.Reason)
	}
}

// printMappers lists the detected mappers, on stderr.
func printMappers(w io.Writer, run *ginRun) {
	for _, m := range run.Mappers {
		def := "no default"
		if m.HasDefault {
			def = "default " + statusStr(m.Default)
		}
		fmt.Fprintf(w, "mapper %s %s [%s]: %s\n", m.Kind, m.Name, def, strings.Join(m.Branches, "; "))
	}
}

func statusStr(s int) string {
	if s == 0 {
		return "unknown"
	}
	return fmt.Sprint(s)
}
