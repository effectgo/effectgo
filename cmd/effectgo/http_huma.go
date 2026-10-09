package main

// The http-huma subcommand. It runs the Huma route analyzer over the packages,
// classifies the variants that reach the HTTP 500 fallback, and prints them.
// Like the openapi subcommand it loads the packages itself and uses the checker
// API, so it can aggregate the routes and the mapper evidence of every package.
//
//	effectgo http-huma [-json] [-all] [-stubs=...] [packages]

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis/checker"

	huma "github.com/faintghost/effectgo/adapters/huma"
	"github.com/faintghost/effectgo/internal/stubs"
)

// runHTTPHuma implements `effectgo http-huma [flags] [packages]`. It returns the
// exit status: 0 on success, 1 when the packages do not load or the analysis
// fails, 2 on a usage error. Findings do not change the status.
func runHTTPHuma(args []string) int {
	fs := flag.NewFlagSet("effectgo http-huma", flag.ContinueOnError)
	fs.StringVar(&stubs.Flag, "stubs", stubs.Flag,
		"comma-separated stub files; falls back to $"+stubs.Env+" when empty")
	jsonOut := fs.Bool("json", false, "print one JSON object per finding (the schema shared with the other HTTP adapters)")
	all := fs.Bool("all", false, "also print info findings (?, new@, foreign and unresolved-kind variants)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	results, err := analyzeHuma(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "effectgo http-huma: %v\n", err)
		return 1
	}
	rep := huma.Classify(results)
	if *jsonOut {
		if err := huma.WriteJSON(os.Stdout, rep, *all); err != nil {
			fmt.Fprintf(os.Stderr, "effectgo http-huma: %v\n", err)
			return 1
		}
		// The coverage goes to stderr so that stdout stays one JSON object per line.
		huma.WriteText(os.Stderr, rep, *all)
		return 0
	}
	huma.WriteText(os.Stdout, rep, *all)
	return 0
}

// analyzeHuma loads the packages, runs the Huma analyzer on them, and returns
// the per-package results of the packages that the patterns name, with every
// position made relative to the working directory.
func analyzeHuma(patterns []string) ([]*huma.Result, error) {
	graph, err := analyzeHTTP(patterns, huma.Analyzer)
	if err != nil {
		return nil, err
	}
	return humaFromGraph(graph, mustGetwd())
}

// humaFromGraph reads the Huma results of an analyzed graph and makes their
// positions relative to cwd.
func humaFromGraph(graph *checker.Graph, cwd string) ([]*huma.Result, error) {
	var out []*huma.Result
	for _, act := range graph.Roots {
		if act.Err != nil {
			return nil, fmt.Errorf("%s: %v", act.Package, act.Err)
		}
		r, ok := act.Result.(*huma.Result)
		if !ok || r == nil {
			continue
		}
		relativizeHuma(r, cwd)
		out = append(out, r)
	}
	return out, nil
}

// mustGetwd returns the working directory, or "" when it cannot be read (the
// positions then stay as the analysis gives them).
func mustGetwd() string {
	cwd, _ := os.Getwd()
	return cwd
}

// relativizeHuma rewrites the positions of a result relative to dir, when they are
// below it.
func relativizeHuma(r *huma.Result, dir string) {
	for i := range r.Routes {
		r.Routes[i].Pos = relPos(r.Routes[i].Pos, dir)
		r.Routes[i].HandlerPos = relPos(r.Routes[i].HandlerPos, dir)
	}
	for i := range r.Mappings {
		r.Mappings[i].Pos = relPos(r.Mappings[i].Pos, dir)
	}
}

// relPos makes the file part of a file:line[:col] position relative to dir.
func relPos(pos, dir string) string {
	if pos == "" || dir == "" {
		return pos
	}
	file := pos
	rest := ""
	if i := strings.Index(pos, ":"); i >= 0 {
		file, rest = pos[:i], pos[i:]
	}
	if rel, err := filepath.Rel(dir, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel + rest
	}
	return pos
}
