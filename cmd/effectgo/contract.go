package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	ginhttp "github.com/effectgo/effectgo/adapters/gin"
	huma "github.com/effectgo/effectgo/adapters/huma"
	"github.com/effectgo/effectgo/internal/hidden"
	"github.com/effectgo/effectgo/internal/stubs"
)

// contractReport is a versioned CLI format, separate from go vet and the legacy
// HTTP finding streams. Rule IDs identify analyzer families, not message text.
type contractReport struct {
	SchemaVersion int                  `json:"schema_version"`
	Diagnostics   []contractDiagnostic `json:"diagnostics"`
	Coverage      contractCoverage     `json:"coverage"`
	ExitCode      int                  `json:"exit_code"`
}

type contractPosition struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type contractDiagnostic struct {
	RuleID   string           `json:"rule_id"`
	Analyzer string           `json:"analyzer"`
	Severity string           `json:"severity"`
	Position contractPosition `json:"position"`
	Message  string           `json:"message"`
	Route    string           `json:"route,omitempty"`
	Handler  string           `json:"handler,omitempty"`
	Variant  string           `json:"variant,omitempty"`
	Status   int              `json:"status,omitzero"`
	Via      string           `json:"via,omitempty"`
}

type contractCoverage struct {
	Packages          int          `json:"packages"`
	HiddenDiagnostics int          `json:"hidden_diagnostics"`
	Gin               httpCoverage `json:"gin"`
	Huma              httpCoverage `json:"huma"`
}

func runContract(args []string) int {
	fs := flag.NewFlagSet("effectgo contract", flag.ContinueOnError)
	format := fs.String("format", "text", "report format: text or json (schema_version 1)")
	all := fs.Bool("all", false, "also include informational HTTP findings")
	fs.StringVar(&stubs.Flag, "stubs", stubs.Flag,
		"comma-separated stub files; falls back to $"+stubs.Env+" when empty")
	bindAnalyzerFlags(fs)
	opts := bindHTTPOptions(fs)
	opts.FailOn = "leak"
	fs.Lookup("fail-on").DefValue = opts.FailOn
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if err := checkFlagOrder("contract", fs, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(os.Stderr, "effectgo contract: invalid format %q (want text or json)\n", *format)
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	report, err := collectContract(patterns, *all, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "effectgo contract: %v\n", err)
		return 1
	}
	if err := writeContract(os.Stdout, report, *format); err != nil {
		fmt.Fprintf(os.Stderr, "effectgo contract: %v\n", err)
		return 1
	}
	if opts.Coverage == "json" {
		if err := writeCoverageJSON(os.Stderr, report.Coverage); err != nil {
			return 1
		}
	} else if *format == "text" {
		fmt.Fprintf(os.Stderr, "effectgo contract: %d packages analysed, %d findings (%d hidden by root rule), %d unresolved HTTP routes\n",
			report.Coverage.Packages, len(report.Diagnostics), report.Coverage.HiddenDiagnostics,
			report.Coverage.Gin.Unresolved+report.Coverage.Huma.Unresolved)
	}
	return report.ExitCode
}

func collectContract(patterns []string, all bool, opts *httpOptions) (contractReport, error) {
	analyzers := make([]*analysis.Analyzer, 0, len(standaloneAnalyzers)+2)
	analyzers = append(analyzers, standaloneAnalyzers...)
	analyzers = append(analyzers, ginhttp.Analyzer, huma.Analyzer)
	graph, err := analyzeHTTP(patterns, analyzers...)
	if err != nil {
		return contractReport{}, err
	}
	httpFindings, sum, err := collectHTTPFromGraph(graph, all)
	if err != nil {
		return contractReport{}, err
	}
	report := contractReport{SchemaVersion: 1, Diagnostics: []contractDiagnostic{}}
	cwd := mustGetwd()
	seen := map[contractDiagnostic]bool{}
	packages := map[string]bool{}
	var pkgTypes []*types.Package
	for _, act := range graph.Roots {
		if !packages[act.Package.ID] {
			packages[act.Package.ID] = true
			pkgTypes = append(pkgTypes, act.Package.Types)
		}
		if !slices.Contains(standaloneAnalyzers, act.Analyzer) {
			continue
		}
		for _, diagnostic := range act.Diagnostics {
			pos := act.Package.Fset.Position(diagnostic.Pos)
			d := contractDiagnostic{
				RuleID: act.Analyzer.Name, Analyzer: act.Analyzer.Name, Severity: "error",
				Position: contractPosition{File: contractFile(pos.Filename, cwd), Line: pos.Line, Column: pos.Column},
				Message:  diagnostic.Message,
			}
			if !seen[d] {
				seen[d] = true
				report.Diagnostics = append(report.Diagnostics, d)
			}
			report.ExitCode = httpGateExitCode
		}
	}
	for _, finding := range httpFindings {
		d := contractDiagnostic{
			RuleID: "http." + finding.Framework, Analyzer: finding.Framework, Severity: finding.Severity,
			Position: contractHTTPPosition(finding.Pos, cwd), Message: finding.Reason,
			Route: finding.Route, Handler: finding.Handler, Variant: finding.Variant,
			Status: finding.Status, Via: finding.Via,
		}
		if !seen[d] {
			seen[d] = true
			report.Diagnostics = append(report.Diagnostics, d)
		}
	}
	slices.SortFunc(report.Diagnostics, func(a, b contractDiagnostic) int {
		return cmp.Or(
			cmp.Compare(a.Position.File, b.Position.File),
			cmp.Compare(a.Position.Line, b.Position.Line),
			cmp.Compare(a.Position.Column, b.Position.Column),
			cmp.Compare(a.RuleID, b.RuleID),
			cmp.Compare(a.Message, b.Message),
			cmp.Compare(a.Route, b.Route),
			cmp.Compare(a.Variant, b.Variant),
			cmp.Compare(a.Status, b.Status),
			cmp.Compare(a.Via, b.Via),
			cmp.Compare(a.Severity, b.Severity),
			cmp.Compare(a.Analyzer, b.Analyzer),
			cmp.Compare(a.Handler, b.Handler),
		)
	})
	report.Coverage = contractCoverage{
		Packages: len(packages), HiddenDiagnostics: hidden.Total("effects", pkgTypes) + hidden.Total("errset", pkgTypes),
		Gin: sum.ginCoverage, Huma: sum.humaCoverage,
	}
	for _, coverage := range []*httpCoverage{&report.Coverage.Gin, &report.Coverage.Huma} {
		for i := range coverage.UnresolvedRoute {
			coverage.UnresolvedRoute[i].Pos = relPos(coverage.UnresolvedRoute[i].Pos, cwd)
		}
		slices.SortFunc(coverage.UnresolvedRoute, func(a, b coverageRoute) int {
			return cmp.Or(cmp.Compare(a.Pos, b.Pos), cmp.Compare(a.Method, b.Method), cmp.Compare(a.Path, b.Path), cmp.Compare(a.Reason, b.Reason))
		})
	}
	counts := map[string]int{}
	for severity, count := range sum.gin {
		counts[severity] += count
	}
	for severity, count := range sum.huma {
		counts[severity] += count
	}
	if gateHTTP(opts, counts, sum.ginCoverage.Unresolved+sum.humaCoverage.Unresolved) != 0 {
		report.ExitCode = httpGateExitCode
	}
	return report, nil
}

func contractFile(file, cwd string) string {
	if filepath.IsAbs(file) && cwd != "" {
		if relative, err := filepath.Rel(cwd, file); err == nil {
			file = relative
		}
	}
	return filepath.ToSlash(file)
}

// Adapters provide file:line or file:line:column. Read numeric suffixes from
// the right so a drive prefix or colon in a filename stays part of the file.
func contractHTTPPosition(pos, cwd string) contractPosition {
	p := contractPosition{File: pos}
	i := strings.LastIndex(pos, ":")
	if i < 0 {
		return p
	}
	last, err := strconv.Atoi(pos[i+1:])
	if err != nil {
		return p
	}
	p.File, p.Line = pos[:i], last
	if j := strings.LastIndex(p.File, ":"); j >= 0 {
		if line, err := strconv.Atoi(p.File[j+1:]); err == nil {
			p.File, p.Line, p.Column = p.File[:j], line, last
		}
	}
	p.File = contractFile(p.File, cwd)
	return p
}

func writeContract(w io.Writer, report contractReport, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(report)
	}
	for _, d := range report.Diagnostics {
		if _, err := fmt.Fprintf(w, "%s:%d:%d: [%s] %s", d.Position.File, d.Position.Line, d.Position.Column, d.RuleID, d.Message); err != nil {
			return err
		}
		if d.Route != "" {
			if _, err := fmt.Fprintf(w, " [%s]", d.Route); err != nil {
				return err
			}
		}
		if d.Variant != "" {
			if _, err := fmt.Fprintf(w, " %s → HTTP %d", d.Variant, d.Status); err != nil {
				return err
			}
		}
		if d.Via != "" {
			if _, err := fmt.Fprintf(w, " (via %s)", d.Via); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}
