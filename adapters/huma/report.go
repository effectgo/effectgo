package huma

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/faintghost/effectgo/errset"
)

// Severities of a finding, most severe first.
const (
	SeverityHigh   = "high"
	SeverityMedium = "medium"
	SeverityInfo   = "info"
)

// Finding is one variant of one route that reaches the 500 fallback. Its JSON
// form is the schema shared with the other framework adapters:
//
//	{"severity","framework","route","handler","pos","variant","status","via","reason"}
type Finding struct {
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

// Stats counts what the classification saw, so a reader can judge coverage.
type Stats struct {
	Routes         int            // every route call that a route rule names
	Resolved       int            // routes with a constant method and path and a resolved handler
	Unresolved     []Route        // the others, with their reasons
	Variants       int            // variants of resolved routes
	Known          int            // variants with a known status
	DefaultUnknown int            // variants whose status is unknown ("default" response)
	Intended500    int            // status-carrying variants with status 500 (a deliberate 500)
	Fallback500    int            // variants answered 500 by the fallback: the findings
	BySeverity     map[string]int // findings per severity
}

// Report is the classification of every route of the analyzed packages.
type Report struct {
	Findings []Finding
	Stats    Stats
}

// Classify turns the per-package results into findings. The evidence is the
// mapper results of every package: a variant that some mapper maps to a status
// below 500 is "mapped elsewhere". The module path decides whether a named
// variant belongs to the project.
func Classify(results []*Result) Report {
	var rep Report
	rep.Stats.BySeverity = map[string]int{}

	module := ""
	evidence := map[string][]string{} // variant key -> "mapper (status N)"
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Module != "" {
			module = r.Module
		}
		for _, m := range r.Mappings {
			if m.Status > 0 && m.Status < 500 {
				evidence[m.Key] = append(evidence[m.Key], fmt.Sprintf("%s maps it to %d", m.Mapper, m.Status))
			}
		}
	}
	for k := range evidence {
		sort.Strings(evidence[k])
		evidence[k] = dedupe(evidence[k])
	}

	for _, r := range results {
		if r == nil {
			continue
		}
		for _, rt := range r.Routes {
			rep.Stats.Routes++
			if rt.Unresolved != "" {
				rep.Stats.Unresolved = append(rep.Stats.Unresolved, rt)
				continue
			}
			rep.Stats.Resolved++
			for _, v := range rt.Variants {
				rep.Stats.Variants++
				if !v.Known {
					rep.Stats.DefaultUnknown++
					continue
				}
				rep.Stats.Known++
				if v.Status != 500 {
					continue
				}
				if v.Carried {
					rep.Stats.Intended500++
					continue
				}
				rep.Stats.Fallback500++
				f := classifyVariant(rt, v, module, evidence)
				rep.Findings = append(rep.Findings, f)
				rep.Stats.BySeverity[f.Severity]++
			}
		}
	}
	sortFindings(rep.Findings)
	return rep
}

// classifyVariant is the severity of one variant that reaches 500.
func classifyVariant(rt Route, v RouteVariant, module string, evidence map[string][]string) Finding {
	f := Finding{
		Framework: "huma",
		Route:     rt.Method + " " + rt.Path,
		Handler:   handlerText(rt),
		Pos:       lineOf(rt.Pos),
		Variant:   v.Display,
		Status:    500,
		Via:       viaText(v.Via),
	}
	named := v.Kind == errset.KindSentinel || v.Kind == errset.KindType
	switch {
	case named && inModule(v.Key, module):
		if ev := evidence[v.Key]; len(ev) > 0 {
			f.Severity = SeverityHigh
			f.Reason = fmt.Sprintf("%s is a project error that is mapped to a non-5xx status elsewhere (%s), but this route returns it without mapping, so Huma answers 500",
				v.Display, strings.Join(ev, "; "))
		} else {
			f.Severity = SeverityMedium
			f.Reason = fmt.Sprintf("%s is a project error that no mapper maps to a status, so this route answers 500 for it", v.Display)
		}
	case named:
		f.Severity = SeverityInfo
		f.Reason = fmt.Sprintf("%s is a foreign error, not a project domain error, and no status is known for it, so Huma answers 500 unless the handler maps it", v.Display)
	case v.Kind == errset.KindOpaque:
		f.Severity = SeverityInfo
		f.Reason = "the error's origin is not known (an unclassified error), and Huma answers 500 for it"
	case v.Kind == errset.KindFresh:
		f.Severity = SeverityInfo
		f.Reason = fmt.Sprintf("the error is created fresh in %s, which no errors.Is or errors.As can match, so Huma answers 500", strings.TrimPrefix(v.Display, "new@"))
	default:
		f.Severity = SeverityInfo
		f.Reason = fmt.Sprintf("%s is not resolved to a concrete error (kind %s), so Huma answers 500 by the fallback", v.Display, v.Kind)
	}
	return f
}

// handlerText is the handler column: the name and the file:line of its literal
// or declaration.
func handlerText(rt Route) string {
	if rt.HandlerPos == "" {
		return rt.Handler
	}
	return rt.Handler + " (" + lineOf(rt.HandlerPos) + ")"
}

// viaText is the witness chain, or "(direct)" when the variant is created in the handler.
func viaText(chain []string) string {
	if len(chain) == 0 {
		return "(direct)"
	}
	return strings.Join(chain, " → ")
}

// inModule reports whether a variant key belongs to the module.
func inModule(key, module string) bool {
	if module == "" {
		return false
	}
	pkg := key
	if i := strings.LastIndex(key, "."); i >= 0 {
		pkg = key[:i]
	}
	return pkg == module || strings.HasPrefix(pkg, module+"/")
}

// lineOf drops the column from a file:line:col position.
func lineOf(pos string) string {
	if strings.Count(pos, ":") >= 2 {
		return pos[:strings.LastIndex(pos, ":")]
	}
	return pos
}

func dedupe(ss []string) []string {
	out := ss[:0]
	for i, s := range ss {
		if i == 0 || s != ss[i-1] {
			out = append(out, s)
		}
	}
	return out
}

var severityRank = map[string]int{SeverityHigh: 0, SeverityMedium: 1, SeverityInfo: 2}

func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		if a.Route != b.Route {
			return a.Route < b.Route
		}
		if a.Pos != b.Pos {
			return a.Pos < b.Pos
		}
		return a.Variant < b.Variant
	})
}

// Shown reports whether a finding is printed: info findings only with all.
func Shown(f Finding, all bool) bool {
	return all || f.Severity != SeverityInfo
}

// WriteJSON writes the shown findings, one JSON object per line.
func WriteJSON(w io.Writer, rep Report, all bool) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, f := range rep.Findings {
		if !Shown(f, all) {
			continue
		}
		if err := enc.Encode(f); err != nil {
			return err
		}
	}
	return nil
}

// WriteText writes the findings grouped by severity, then the coverage.
func WriteText(w io.Writer, rep Report, all bool) {
	for _, sev := range []string{SeverityHigh, SeverityMedium, SeverityInfo} {
		n := rep.Stats.BySeverity[sev]
		if sev == SeverityInfo && !all {
			if n > 0 {
				fmt.Fprintf(w, "== info: %d hidden (use -all)\n", n)
			}
			continue
		}
		fmt.Fprintf(w, "== %s: %d\n", sev, n)
		for _, f := range rep.Findings {
			if f.Severity != sev {
				continue
			}
			fmt.Fprintf(w, "  %s  %s  %s\n", f.Route, f.Handler, f.Pos)
			fmt.Fprintf(w, "    variant %s -> %d  via %s\n", f.Variant, f.Status, f.Via)
			fmt.Fprintf(w, "    %s\n", f.Reason)
		}
	}
	writeCoverage(w, rep.Stats)
}

// writeCoverage prints the route and variant counts.
func writeCoverage(w io.Writer, s Stats) {
	fmt.Fprintf(w, "routes: %d found, %d resolved, %d unresolved\n", s.Routes, s.Resolved, len(s.Unresolved))
	for _, r := range s.Unresolved {
		fmt.Fprintf(w, "  unresolved: %s %s (%s): %s\n", strings.ToUpper(r.Method), r.Path, lineOf(r.Pos), r.Unresolved)
	}
	fmt.Fprintf(w, "variants: %d; %d with a known status, %d default (unknown); %d intended 500 (carried); %d answered 500 by the fallback\n",
		s.Variants, s.Known, s.DefaultUnknown, s.Intended500, s.Fallback500)
	fmt.Fprintf(w, "findings: high %d, medium %d, info %d\n",
		s.BySeverity[SeverityHigh], s.BySeverity[SeverityMedium], s.BySeverity[SeverityInfo])
}
