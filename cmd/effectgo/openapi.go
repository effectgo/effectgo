package main

// The openapi subcommand. The other modes run as a multichecker, which exits
// the process. This one loads the packages itself and runs the openapi analyzer
// through the checker API, so it can aggregate the routes of every package.

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/faintghost/effectgo/internal/stubs"
	"github.com/faintghost/effectgo/openapi"
)

// runOpenAPI implements `effectgo openapi [flags] [packages]`. It returns the
// exit status: 0 on success, 1 when -check finds a missing code or the run
// fails, 2 on a usage error.
func runOpenAPI(args []string) int {
	fs := flag.NewFlagSet("effectgo openapi", flag.ContinueOnError)
	bindEngine(fs)
	fs.StringVar(&stubs.Flag, "stubs", stubs.Flag,
		"comma-separated stub files; falls back to $"+stubs.Env+" when empty")
	mergePath := fs.String("merge", "", "merge the error responses into this OpenAPI 3 JSON document and print the result")
	checkPath := fs.String("check", "", "compare the error responses with this OpenAPI 3 JSON document; exit 1 on a missing code")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := checkFlagOrder("openapi", fs, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *mergePath != "" && *checkPath != "" {
		fmt.Fprintln(os.Stderr, "effectgo openapi: -merge and -check cannot be combined")
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	ops, undocumented, err := collect(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "effectgo openapi: %v\n", err)
		return 1
	}
	switch {
	case *checkPath != "":
		doc, err := readDoc(*checkPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "effectgo openapi: %v\n", err)
			return 1
		}
		return printCheck(os.Stdout, openapi.Check(doc, ops, undocumented), ops)
	case *mergePath != "":
		doc, err := readDoc(*mergePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "effectgo openapi: %v\n", err)
			return 1
		}
		printUndocumented(undocumented)
		merged, notes := openapi.Merge(doc, ops)
		for _, n := range notes {
			fmt.Fprintln(os.Stderr, n)
		}
		return printJSON(os.Stdout, merged)
	}
	printUndocumented(undocumented)
	return printJSON(os.Stdout, openapi.Fragment(ops))
}

// collect loads the packages, runs the openapi analyzer on them, and groups the
// routes into operations. The operations are sorted by path and method.
func collect(patterns []string) ([]*openapi.Op, []openapi.Route, error) {
	// NeedModule gives the analysis passes their module, which errset uses to
	// collapse foreign unexported variants. Tests are off by default.
	cfg := &packages.Config{Mode: packages.LoadAllSyntax | packages.NeedModule}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, nil, fmt.Errorf("the packages do not load")
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{openapi.Analyzer}, pkgs, nil)
	if err != nil {
		return nil, nil, err
	}
	var routes []openapi.Route
	for _, act := range graph.Roots {
		if act.Err != nil {
			return nil, nil, fmt.Errorf("%s: %v", act.Package, act.Err)
		}
		if r, ok := act.Result.(openapi.Result); ok {
			routes = append(routes, r...)
		}
	}
	ops, undocumented := openapi.Operations(routes)
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Path != ops[j].Path {
			return ops[i].Path < ops[j].Path
		}
		return ops[i].Method < ops[j].Method
	})
	return ops, undocumented, nil
}

// readDoc reads an OpenAPI JSON document. Numbers are kept as written.
func readDoc(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if doc == nil {
		return nil, fmt.Errorf("%s: not a JSON object", path)
	}
	return doc, nil
}

// printUndocumented lists the routes that have no error docs, on stderr.
func printUndocumented(routes []openapi.Route) {
	for _, r := range routes {
		fmt.Fprintf(os.Stderr, "undocumented: %s %s (%s): %s\n", strings.ToUpper(r.Method), r.Path, r.Pos, r.Undocumented)
	}
}

func printJSON(w io.Writer, v any) int {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "effectgo openapi: %v\n", err)
		return 1
	}
	fmt.Fprintln(w, string(out))
	return 0
}

// printCheck writes the drift report per operation and returns the exit status:
// 1 when any code is missing from the document.
func printCheck(w io.Writer, drifts []openapi.Drift, ops []*openapi.Op) int {
	byOp := map[string][]openapi.Drift{}
	for _, d := range drifts {
		byOp[d.Op] = append(byOp[d.Op], d)
	}
	blocked := false
	counts := map[string]int{}
	for _, op := range ops {
		name := strings.ToUpper(op.Method) + " " + op.Path
		ds := byOp[name]
		if len(ds) == 0 {
			fmt.Fprintf(w, "%s: ok\n", name)
			continue
		}
		fmt.Fprintf(w, "%s:\n", name)
		for _, d := range ds {
			counts[d.Kind]++
			if d.Blocks {
				blocked = true
			}
			code := ""
			if d.Code != "" {
				code = " " + d.Code
			}
			fmt.Fprintf(w, "  %s%s: %s\n", d.Kind, code, d.Detail)
		}
	}
	for _, d := range drifts {
		if d.Kind == "unchecked" || d.Kind == "not in document" {
			if _, inOps := opIndex(ops, d.Op); !inOps {
				counts[d.Kind]++
				fmt.Fprintf(w, "%s: %s: %s\n", d.Op, d.Kind, d.Detail)
			}
		}
	}
	fmt.Fprintf(w, "summary: %d missing, %d stale, %d not in document, %d unchecked\n",
		counts["missing"], counts["stale"], counts["not in document"], counts["unchecked"])
	if blocked {
		return 1
	}
	return 0
}

// opIndex reports whether an operation named "METHOD /path" is among ops.
func opIndex(ops []*openapi.Op, name string) (int, bool) {
	for i, op := range ops {
		if strings.ToUpper(op.Method)+" "+op.Path == name {
			return i, true
		}
	}
	return -1, false
}
