package ginhttp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	ginhttp "github.com/faintghost/effectgo/adapters/gin"
	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
)

// run analyzes the fixture packages with the shipped gin stubs and the project
// stubs of the fixture, and returns the result of the handlers package.
func run(t *testing.T, pkg string) ginhttp.Result {
	t.Helper()
	dir := analysistest.TestData()
	old := stubs.Flag
	stubs.Flag = strings.Join([]string{
		filepath.Join(dir, "..", "..", "..", "stubs", "gin.stubs"),
		filepath.Join(dir, "stubs", "project.stubs"),
	}, ",")
	t.Cleanup(func() { stubs.Flag = old })
	results := analysistest.Run(t, dir, ginhttp.Analyzer, pkg)
	if len(results) != 1 {
		t.Fatalf("want one result, got %d", len(results))
	}
	res, ok := results[0].Result.(ginhttp.Result)
	if !ok {
		t.Fatalf("result has type %T", results[0].Result)
	}
	return res
}

// find returns the findings of a kind whose handler name contains name.
func find(res ginhttp.Result, kind, name string) []ginhttp.Finding {
	var out []ginhttp.Finding
	for _, f := range res.Findings {
		if f.Kind == kind && strings.Contains(f.Handler, name) {
			out = append(out, f)
		}
	}
	return out
}

// has reports whether a finding has the variant (by display name) and status.
func has(fs []ginhttp.Finding, variant string, status int) bool {
	for _, f := range fs {
		if f.Variant == variant && f.Status == status {
			return true
		}
	}
	return false
}

func TestMappersDetected(t *testing.T) {
	res := run(t, "fixture/handlers")
	kinds := map[string]string{}
	for _, m := range res.Mappers {
		kinds[strings.SplitN(m.Name, " ", 2)[0]] = m.Kind
	}
	if kinds["storeError"] != "sink" {
		t.Errorf("storeError: want a sink mapper, got mappers %v", res.Mappers)
	}
	if kinds["statusOf"] != "status" {
		t.Errorf("statusOf: want a status mapper, got mappers %v", res.Mappers)
	}
	for _, m := range res.Mappers {
		if strings.HasPrefix(m.Name, "storeError") && (!m.HasDefault || m.Default != 500) {
			t.Errorf("storeError default: want 500, got %+v", m)
		}
	}
}

func TestSinkMapperCallReachesDefault(t *testing.T) {
	res := run(t, "fixture/handlers")
	fs := find(res, "reach", "GetItem")
	if !has(fs, "store.ErrInvalid", 500) {
		t.Errorf("GetItem: ErrInvalid should reach the mapper's default 500, got %+v", fs)
	}
	if has(fs, "store.ErrNotFound", 500) {
		t.Errorf("GetItem: ErrNotFound has a 404 branch in storeError, must not reach 500")
	}
}

func TestInlineDefaultReachesFiveHundred(t *testing.T) {
	res := run(t, "fixture/handlers")
	if fs := find(res, "reach", "Create"); !has(fs, "store.ErrConflict", 500) {
		t.Errorf("Create: ErrConflict should reach an inline 500, got %+v", fs)
	}
	if fs := find(res, "reach", "Unlock"); !has(fs, "store.ErrLocked", 500) {
		t.Errorf("Unlock: ErrLocked should reach an inline 500, got %+v", fs)
	}
}

func TestStatusMapperFallthrough(t *testing.T) {
	res := run(t, "fixture/handlers")
	fs := find(res, "reach", "Lookup")
	if !has(fs, "store.ErrNotFound", 500) {
		t.Errorf("Lookup: ErrNotFound should take the fallthrough 500, got %+v", fs)
	}
	if has(fs, "store.ErrInvalid", 500) {
		t.Errorf("Lookup: ErrInvalid is mapped to 400 by statusOf, must not reach 500")
	}
}

func TestErrorsIsElseBranch(t *testing.T) {
	res := run(t, "fixture/handlers")
	fs := find(res, "reach", "Delete")
	if !has(fs, "store.ErrInvalid", 500) {
		t.Errorf("Delete: the else 500 should reach ErrInvalid, got %+v", fs)
	}
	if has(fs, "store.ErrNotFound", 500) {
		t.Errorf("Delete: ErrNotFound is matched by errors.Is to 404, must not reach 500")
	}
}

func TestFreshErrorKind(t *testing.T) {
	res := run(t, "fixture/handlers")
	fs := find(res, "reach", "Fresh")
	if len(fs) == 0 {
		t.Fatalf("Fresh: want a finding for the fresh error, got none")
	}
	for _, f := range fs {
		if f.ErrKind != "fresh" || f.Key != "" {
			t.Errorf("Fresh: want an unnamed fresh variant, got %+v", f)
		}
	}
}

func TestLeaks(t *testing.T) {
	res := run(t, "fixture/handlers")
	leaks := find(res, "leak", "Create")
	if len(leaks) != 1 || leaks[0].Status != 500 || leaks[0].Variant != "err" {
		t.Errorf("Create: want one 500 leak of err, got %+v", leaks)
	}
	leaks = find(res, "leak", "Nested")
	if len(leaks) != 1 || leaks[0].Status != 500 {
		t.Errorf("Nested: want one 500 leak two literals deep, got %+v", leaks)
	}
	if got := find(res, "leak", "Lookup"); len(got) != 0 {
		t.Errorf("Lookup: a constant body must not leak, got %+v", got)
	}
	leaks = find(res, "leak", "Unlock")
	if len(leaks) != 1 || leaks[0].Status != 400 {
		t.Errorf("Unlock: want one 400 leak through the variadic values of c.String, got %+v", leaks)
	}
}

func TestRoutesJoinFindings(t *testing.T) {
	res := run(t, "fixture/handlers")
	want := map[string]bool{"GET /api/items/1": false, "POST /items": false, "DELETE /items/:id": false}
	for _, r := range res.Routes {
		key := r.Method + " " + r.Path
		if _, ok := want[key]; !ok {
			t.Errorf("unexpected route %q", key)
			continue
		}
		want[key] = true
		if r.HandlerPos == "" {
			t.Errorf("route %q has no handler position", key)
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("route %q not resolved", k)
		}
	}
}

func TestNoFindingForStatusOnly(t *testing.T) {
	res := run(t, "fixture/handlers")
	for _, f := range res.Findings {
		if strings.Contains(f.Handler, "storeError") && f.Kind == "reach" {
			t.Errorf("a mapper's own parameter must not produce a finding: %+v", f)
		}
	}
}

// TestMain runs the adapter tests with the error-set engine named by
// EFFECTGO_TEST_ENGINE (ast or ssa); the default is the SSA engine.
func TestMain(m *testing.M) {
	if e := os.Getenv("EFFECTGO_TEST_ENGINE"); e != "" {
		if err := errset.Analyzer.Flags.Set("engine", e); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}
