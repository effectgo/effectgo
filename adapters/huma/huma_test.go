package huma_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/faintghost/effectgo/adapters/huma"
	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
)

// fixtureOnce memoizes the one analysis of the fixture: analysistest analyzes
// every dependency of the fixture (the whole stdlib closure it imports), which
// takes about a minute, so the tests share one run.
var (
	fixtureOnce sync.Once
	fixtureRes  *huma.Result
	fixtureErr  error
)

// fixtureResult runs the adapter on the fixture package app with the shipped
// huma stub file. The fixture is GOPATH-style, so the pass has no module; the
// classification test sets the module by hand on its own copy.
func fixtureResult(t *testing.T) *huma.Result {
	t.Helper()
	fixtureOnce.Do(func() {
		fixtureRes, fixtureErr = runFixture(t)
	})
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
	return fixtureRes
}

func runFixture(t *testing.T) (*huma.Result, error) {
	td, err := filepath.Abs(analysistest.TestData())
	if err != nil {
		return nil, err
	}
	stubs.Flag = filepath.Join(td, "..", "..", "..", "stubs", "huma.stubs")
	results := analysistest.Run(t, td, huma.Analyzer, "app")
	if len(results) != 1 {
		return nil, fmt.Errorf("want one result, got %d", len(results))
	}
	r, ok := results[0].Result.(*huma.Result)
	if !ok {
		return nil, fmt.Errorf("result has type %T, want *huma.Result", results[0].Result)
	}
	return r, nil
}

// classifiedFixture classifies the fixture with the module set to "app" (the
// GOPATH fixture has no module of its own). The copy keeps the shared result as it was.
func classifiedFixture(t *testing.T) huma.Report {
	t.Helper()
	r := *fixtureResult(t)
	r.Module = "app"
	return huma.Classify([]*huma.Result{&r})
}

// route returns the route with the given method and path.
func route(t *testing.T, r *huma.Result, method, path string) huma.Route {
	t.Helper()
	for _, rt := range r.Routes {
		if rt.Method == method && rt.Path == path {
			return rt
		}
	}
	t.Fatalf("no route %s %s; have %d routes", method, path, len(r.Routes))
	return huma.Route{}
}

// variant returns the variant of rt with the given display name.
func variant(t *testing.T, rt huma.Route, display string) huma.RouteVariant {
	t.Helper()
	for _, v := range rt.Variants {
		if v.Display == display {
			return v
		}
	}
	t.Fatalf("route %s %s has no variant %q; have %+v", rt.Method, rt.Path, display, rt.Variants)
	return huma.RouteVariant{}
}

func TestRoutesAndStatuses(t *testing.T) {
	r := fixtureResult(t)
	if got := len(r.Routes); got != 15 {
		t.Fatalf("routes found: got %d, want 15 (every route call of the fixture)", got)
	}

	// A raw sentinel from a dependency: the default errmap gives 500.
	raw := route(t, r, "GET", "/raw-sentinel/{id}")
	if raw.Unresolved != "" {
		t.Fatalf("raw route unresolved: %s", raw.Unresolved)
	}
	if v := variant(t, raw, "service.ErrNotFound"); v.Status != 500 || !v.Known || v.Carried {
		t.Errorf("ErrNotFound raw: got %+v, want status 500 known, not carried", v)
	}

	// A typed domain error and a sentinel from the same call.
	save := route(t, r, "POST", "/save/{id}")
	if v := variant(t, save, "service.ErrConflict"); v.Status != 500 {
		t.Errorf("ErrConflict: status %d, want 500", v.Status)
	}
	if v := variant(t, save, "service.ValidationError"); v.Status != 500 {
		t.Errorf("ValidationError: status %d, want 500 (no StatusError)", v.Status)
	}

	// mapErr's branches: the constructors keep the status they carry, so the
	// mapped route's variants are 400, 404 and 500 with known statuses.
	mapped := route(t, r, "GET", "/mapped/{id}")
	for _, want := range []struct {
		display string
		status  int
	}{
		{"huma.Error400BadRequest", 400},
		{"huma.Error404NotFound", 404},
		{"huma.Error500InternalServerError", 500},
	} {
		v := variant(t, mapped, want.display)
		if v.Status != want.status || !v.Known || !v.Carried {
			t.Errorf("%s: got %+v, want status %d known and carried", want.display, v, want.status)
		}
	}

	// A status-carrying error that a %w wrap hides still keeps its status: the
	// framework's errors.As finds it, so the route answers 404.
	wrapped := route(t, r, "PUT", "/wrapped-status")
	if v := variant(t, wrapped, "huma.Error404NotFound"); v.Status != 404 || !v.Known {
		t.Errorf("%%w-wrapped Error404NotFound: got %+v, want status 404 known", v)
	}

	fresh := route(t, r, "DELETE", "/fresh")
	if v := variant(t, fresh, "new@app.Register"); v.Kind != errset.KindFresh || v.Status != 500 {
		t.Errorf("fresh error: got %+v, want kind fresh, status 500", v)
	}

	// huma.Register with a literal operation gets its method and path from it.
	reg := route(t, r, "PATCH", "/register-literal")
	if reg.Unresolved != "" {
		t.Errorf("register literal unresolved: %s", reg.Unresolved)
	}
}

func TestUnresolvedRoutes(t *testing.T) {
	r := fixtureResult(t)
	// The dynamic path keeps its method (GET); the Register whose operation is a
	// variable has no method, because it is not known.
	cases := []struct {
		method, reason string
	}{
		{"GET", "the path is not a constant string"},
		{"", "argument 1 is not a composite literal"},
	}
	var unresolved []huma.Route
	for _, rt := range r.Routes {
		if rt.Unresolved != "" {
			unresolved = append(unresolved, rt)
		}
	}
	if len(unresolved) != 2 {
		t.Fatalf("unresolved routes: got %d, want 2: %+v", len(unresolved), unresolved)
	}
	for _, c := range cases {
		found := false
		for _, rt := range unresolved {
			if strings.Contains(rt.Unresolved, c.reason) && rt.Method == c.method {
				found = true
			}
		}
		if !found {
			t.Errorf("no unresolved route with reason %q; have %+v", c.reason, unresolved)
		}
	}
}

func TestStatusCarriers(t *testing.T) {
	r := fixtureResult(t)

	// NewError with a constant status: the status is the constant argument.
	c := route(t, r, "GET", "/constant-status")
	if v := variant(t, c, "huma.NewError(409)"); v.Status != 409 || !v.Known || !v.Carried {
		t.Errorf("NewError(409): got %+v", v)
	}

	// NewError with a status parameter: unknown, so it is not a 500 finding.
	d := route(t, r, "GET", "/dynamic-status")
	if v := variant(t, d, "huma.NewError(?)"); v.Known {
		t.Errorf("NewError(?): got %+v, want an unknown status", v)
	}

	// A type with GetStatus: the framework reads the status, which is not known.
	g := route(t, r, "GET", "/get-status")
	if v := variant(t, g, "app.getStatusErr"); v.Known {
		t.Errorf("GetStatus type: got %+v, want an unknown status", v)
	}

	// A type with StatusCode only: Huma does not read StatusCode, so it is 500.
	s := route(t, r, "GET", "/status-code")
	if v := variant(t, s, "app.codeErr"); v.Status != 500 || !v.Known || v.Carried {
		t.Errorf("StatusCode type: got %+v, want 500 known and not carried", v)
	}
}

func TestClassifyFixture(t *testing.T) {
	rep := classifiedFixture(t)

	want := map[string]string{
		// route + " " + variant -> severity
		"GET /raw-sentinel/{id} service.ErrNotFound":  huma.SeverityHigh,
		"POST /save/{id} service.ValidationError":     huma.SeverityHigh,
		"POST /save/{id} service.ErrConflict":         huma.SeverityHigh, // mapped to 409 by the guard in PUT /conflict-guard
		"GET /predicate-raw service.BadInput":         huma.SeverityHigh, // mapped to 400 by IsBad in PUT /predicate-guard
		"PATCH /register-literal service.ErrNotFound": huma.SeverityHigh,
		"DELETE /fresh new@app.Register":              huma.SeverityInfo,
		"GET /status-code app.codeErr":                huma.SeverityMedium,
	}
	got := map[string]string{}
	for _, f := range rep.Findings {
		got[f.Route+" "+f.Variant] = f.Severity
	}
	for k, sev := range want {
		if got[k] != sev {
			t.Errorf("%s: severity %q, want %q", k, got[k], sev)
		}
	}
	if len(got) != len(want) {
		t.Errorf("findings: got %d, want %d: %v", len(got), len(want), got)
	}
	if rep.Stats.Intended500 == 0 {
		t.Errorf("no intended 500 counted: the carried Error500 of mapErr should be one")
	}
	// Exactly two variants have an unknown status: NewError with a status
	// parameter, and the GetStatus type. Neither is a 500 finding.
	if rep.Stats.DefaultUnknown != 2 {
		t.Errorf("unknown-status variants: got %d, want 2 (NewError(?) and the GetStatus type)", rep.Stats.DefaultUnknown)
	}
}

// TestJSONSchema checks the shared output schema: one object per line, with the
// nine keys in order, and info findings only with -all.
func TestJSONSchema(t *testing.T) {
	rep := classifiedFixture(t)

	var b strings.Builder
	if err := huma.WriteJSON(&b, rep, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != rep.Stats.BySeverity["high"]+rep.Stats.BySeverity["medium"] {
		t.Fatalf("lines without -all: got %d, want high+medium", len(lines))
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, `{"severity":"`) {
			t.Errorf("line does not start with the severity key: %s", line)
		}
		keys := []string{`"severity"`, `"framework":"huma"`, `"route"`, `"handler"`, `"pos"`, `"variant"`, `"status":500`, `"via"`, `"reason"`}
		last := -1
		for _, k := range keys {
			i := strings.Index(line, k)
			if i <= last {
				t.Errorf("key %s missing or out of order in %s", k, line)
			}
			last = i
		}
		if strings.Contains(line, "info") && !strings.Contains(line, `"severity":"info"`) {
			t.Errorf("unexpected info text: %s", line)
		}
	}

	var all strings.Builder
	_ = huma.WriteJSON(&all, rep, true)
	if n := strings.Count(all.String(), "\n"); n != len(rep.Findings) {
		t.Errorf("with -all: %d lines, want %d", n, len(rep.Findings))
	}
}
