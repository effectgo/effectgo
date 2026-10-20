package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	huma "github.com/faintghost/effectgo/adapters/huma"
)

const httpGateExitCode = 3

type httpOptions struct {
	FailOn         string
	FailUnresolved bool
	Coverage       string
}

func bindHTTPOptions(fs *flag.FlagSet) *httpOptions {
	o := &httpOptions{FailOn: "none"}
	fs.Var((*severityFlag)(&o.FailOn), "fail-on", "failure threshold: none, high, medium, leak or info")
	fs.BoolVar(&o.FailUnresolved, "fail-unresolved", false, "fail when route coverage has unresolved routes")
	fs.Var((*coverageFlag)(&o.Coverage), "coverage", "coverage output: json")
	return o
}

type severityFlag string

func (f *severityFlag) String() string { return string(*f) }

func (f *severityFlag) Set(v string) error {
	if _, ok := severityOrder[v]; v != "none" && !ok {
		return fmt.Errorf("invalid severity %q (want none, high, medium, leak or info)", v)
	}
	*f = severityFlag(v)
	return nil
}

type coverageFlag string

func (f *coverageFlag) String() string { return string(*f) }

func (f *coverageFlag) Set(v string) error {
	if v != "json" {
		return fmt.Errorf("invalid coverage format %q (want json)", v)
	}
	*f = coverageFlag(v)
	return nil
}

func gateHTTP(opts *httpOptions, counts map[string]int, unresolved int) int {
	if opts.FailUnresolved && unresolved > 0 {
		return httpGateExitCode
	}
	threshold := rankOf(opts.FailOn)
	if opts.FailOn == "none" {
		return 0
	}
	for severity, count := range counts {
		if count > 0 && rankOf(severity) <= threshold {
			return httpGateExitCode
		}
	}
	return 0
}

func writeCoverageJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

type coverageRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Pos    string `json:"pos"`
	Reason string `json:"reason"`
}

type httpCoverage struct {
	Framework       string          `json:"framework"`
	Handlers        int             `json:"handlers,omitempty"`
	Mappers         int             `json:"mappers,omitempty"`
	Routes          int             `json:"routes"`
	Resolved        int             `json:"resolved"`
	Unresolved      int             `json:"unresolved"`
	Variants        int             `json:"variants,omitempty"`
	Known           int             `json:"known,omitempty"`
	DefaultUnknown  int             `json:"default_unknown,omitempty"`
	Intended500     int             `json:"intended_500,omitempty"`
	Fallback500     int             `json:"fallback_500,omitempty"`
	Findings        map[string]int  `json:"findings"`
	UnresolvedRoute []coverageRoute `json:"unresolved_routes,omitempty"`
	Limit           string          `json:"limit,omitempty"`
}

func humaCoverage(rep huma.Report) httpCoverage {
	return httpCoverage{
		Framework:       "huma",
		Routes:          rep.Stats.Routes,
		Resolved:        rep.Stats.Resolved,
		Unresolved:      len(rep.Stats.Unresolved),
		Variants:        rep.Stats.Variants,
		Known:           rep.Stats.Known,
		DefaultUnknown:  rep.Stats.DefaultUnknown,
		Intended500:     rep.Stats.Intended500,
		Fallback500:     rep.Stats.Fallback500,
		Findings:        rep.Stats.BySeverity,
		UnresolvedRoute: humaUnresolvedRoutes(rep.Stats.Unresolved),
	}
}

func humaUnresolvedRoutes(routes []huma.Route) []coverageRoute {
	out := make([]coverageRoute, 0, len(routes))
	for _, r := range routes {
		out = append(out, coverageRoute{Method: r.Method, Path: r.Path, Pos: r.Pos, Reason: r.Unresolved})
	}
	return out
}

func ginCoverage(run *ginRun, counts map[string]int) httpCoverage {
	unresolved := make([]coverageRoute, 0, run.Unresolved)
	for _, r := range run.Routes {
		if r.PathUnresolved != "" {
			unresolved = append(unresolved, coverageRoute{Method: r.Method, Path: r.Path, Pos: r.Pos, Reason: r.PathUnresolved})
		}
	}
	return httpCoverage{
		Framework:       "gin",
		Limit:           "Gin route counts include only registrations with resolved handlers; variable handlers and wrappers can be outside coverage",
		Handlers:        run.Handlers,
		Mappers:         len(run.Mappers),
		Routes:          len(run.Routes),
		Resolved:        len(run.Routes) - len(unresolved),
		Unresolved:      len(unresolved),
		Findings:        counts,
		UnresolvedRoute: unresolved,
	}
}

func httpUnionCoverage(sum httpSummary) map[string]any {
	return map[string]any{
		"framework": "http",
		"gin":       sum.ginCoverage,
		"huma":      sum.humaCoverage,
	}
}
