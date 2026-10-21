package main

// Integration tests of the P2 merge (Huma + Gin). They run the real pipeline on
// the fixture module of testdata/integrate: an app with one Gin handler and one
// Huma route that pass the same Huma carried 404 through local variables, a
// database/sql fixture, and the adapters' fake Huma and Gin packages.

import (
	"go/ast"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	ginhttp "github.com/effectgo/effectgo/adapters/gin"
	huma "github.com/effectgo/effectgo/adapters/huma"
	"github.com/effectgo/effectgo/effects"
	"github.com/effectgo/effectgo/errset"
	"github.com/effectgo/effectgo/internal/stubs"
)

// stubsDir is the absolute path of the repository's stubs/ directory, taken when
// the package starts (before any test enters the fixture module).
var stubsDir, _ = filepath.Abs(filepath.Join("..", "..", "stubs"))

// fixtureModule enters the fixture module (the packages' patterns are relative to it).
func fixtureModule(t *testing.T) {
	t.Helper()
	t.Chdir(filepath.Join("testdata", "integrate"))
}

// shippedStubs returns the absolute paths of the shipped stub files (stubs/*.stubs).
func shippedStubs(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(stubsDir, "*.stubs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 6 {
		t.Fatalf("want the six shipped stub files, got %v", files)
	}
	return files
}

// useStubs sets the shared -stubs value for one test.
func useStubs(t *testing.T, files ...string) {
	t.Helper()
	old := stubs.Flag
	stubs.Flag = strings.Join(files, ",")
	t.Cleanup(func() { stubs.Flag = old })
}

// findFunc returns the declaration of the named function in the package's files.
func findFunc(t *testing.T, pkg *packages.Package, name string) *ast.FuncDecl {
	t.Helper()
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == name {
				return fd
			}
		}
	}
	t.Fatalf("no function %s in %s", name, pkg.PkgPath)
	return nil
}

// rootOf returns the root action of the analyzer for the package whose path ends in suffix.
func rootOf(t *testing.T, graph *checker.Graph, analyzer string, suffix string) *checker.Action {
	t.Helper()
	for _, act := range graph.Roots {
		if act.Analyzer.Name == analyzer && strings.HasSuffix(act.Package.PkgPath, suffix) {
			return act
		}
	}
	t.Fatalf("no %s action for %s", analyzer, suffix)
	return nil
}

// TestInteractionCarriedThroughLocals is interaction (a). A Huma carried variant
// that passes through local error variables keeps its Carried flag and its status
// in errset's flow view, which the gin adapter reads. The gin adapter names the
// Huma variant in its finding, and the Huma adapter knows its status (404), so
// it reports no 5xx finding for that route.
func TestInteractionCarriedThroughLocals(t *testing.T) {
	fixtureModule(t)
	useStubs(t, shippedStubs(t)...)

	// The flow view of errset: Find returns e2, two assignments after the constructor.
	graph, err := analyzeHTTP([]string{"./app"}, errset.Analyzer)
	if err != nil {
		t.Fatal(err)
	}
	act := rootOf(t, graph, errset.Analyzer.Name, "/app")
	res := act.Result.(errset.Result)
	fd := findFunc(t, act.Package, "Find")
	var ret *ast.Ident
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if r, ok := n.(*ast.ReturnStmt); ok && len(r.Results) == 2 {
			ret, _ = r.Results[1].(*ast.Ident)
		}
		return true
	})
	if ret == nil || ret.Name != "e2" {
		t.Fatalf("want the return of e2 in Find, got %v", ret)
	}
	got := res.Flows[fd].Of(ret)
	if len(got) != 1 || !got[0].Carried || got[0].Status != 404 || !strings.Contains(got[0].Name, "Error404NotFound") {
		t.Fatalf("flow view of e2 lost the carried status: %+v", got)
	}

	// The two adapters on the same load.
	out, sum, err := collectHTTP([]string{"./app"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var ginSeen, humaSeen bool
	for _, f := range out {
		if strings.Contains(f.Variant, "Error404NotFound") {
			switch f.Framework {
			case "gin":
				ginSeen = f.Route == "GET /through" && f.Status == 500 && f.Severity != "info"
			case "huma":
				humaSeen = true
			}
		}
	}
	if !ginSeen {
		t.Errorf("gin: want a finding for GET /through naming huma.Error404NotFound at 500, got %+v", out)
	}
	if humaSeen {
		t.Errorf("huma: the carried 404 has its own status, so no finding is expected: %+v", out)
	}
	if sum.humaRoutes != 1 || sum.humaResolved != 1 {
		t.Errorf("huma routes: want 1 found and resolved, got %d and %d", sum.humaRoutes, sum.humaResolved)
	}
}

// TestRowsScanStubFix checks the stub fix on database/sql: (*sql.Row).Scan returns
// sql.ErrNoRows (sql.go:3513), (*sql.Rows).Scan does not (sql.go:3365-3386: it
// returns rs.lasterr, the closed-rows error or a conversion error). pgx's Rows.Scan
// has no errors rule either, and its Row rule is the only one with ErrNoRows
// (pgx v5.9.2 rows.go:111 and 235); see TestPgxRowsStubRules.
func TestRowsScanStubFix(t *testing.T) {
	fixtureModule(t)
	useStubs(t, shippedStubs(t)[stdlibIndex(t)])
	graph, err := analyzeHTTP([]string{"./rowscan"}, errset.Analyzer)
	if err != nil {
		t.Fatal(err)
	}
	res := rootOf(t, graph, errset.Analyzer.Name, "/rowscan").Result.(errset.Result)
	cases := map[string]bool{"RowsScan": false, "RowScan": true}
	for fn, want := range cases {
		got := false
		for f, fe := range res.Funcs {
			if f.Name() != fn {
				continue
			}
			for _, v := range fe.Variants {
				if v.Key == "database/sql.ErrNoRows" {
					got = true
				}
			}
		}
		if got != want {
			t.Errorf("%s: sql.ErrNoRows in its set = %v, want %v", fn, got, want)
		}
	}
}

// stdlibIndex is the position of stdlib.stubs among the shipped stub files.
func stdlibIndex(t *testing.T) int {
	t.Helper()
	for i, f := range shippedStubs(t) {
		if filepath.Base(f) == "stdlib.stubs" {
			return i
		}
	}
	t.Fatal("no stdlib.stubs")
	return -1
}

// TestPgxRowsStubRules parses the shipped pgx.stubs: the errors rule of pgx.Row
// names ErrNoRows, and no errors rule of pgx.Rows.Scan does.
func TestPgxRowsStubRules(t *testing.T) {
	path := filepath.Join(stubsDir, "pgx.stubs")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := stubs.Parse(path, data)
	if err != nil {
		t.Fatal(err)
	}
	rowErr, rowsErr := "", false
	for _, r := range rules {
		if r.Verb != stubs.VerbErrors {
			continue
		}
		switch r.Target.String() {
		case "(github.com/jackc/pgx/v5.Row).Scan":
			rowErr = r.Value
		case "(github.com/jackc/pgx/v5.Rows).Scan":
			rowsErr = true
		}
	}
	if !strings.Contains(rowErr, "github.com/jackc/pgx/v5.ErrNoRows") {
		t.Errorf("pgx.Row.Scan errors rule %q lacks ErrNoRows", rowErr)
	}
	if rowsErr {
		t.Errorf("pgx.Rows.Scan has an errors rule; it must not name ErrNoRows")
	}
}

// TestShippedStubsLoadTogether is interaction (c). All six shipped stub files
// load together, through EFFECTGO_STUBS as well as -stubs, with no error in any
// analyzer (duplicate rules and malformed lines are errors) on the fixture app.
func TestShippedStubsLoadTogether(t *testing.T) {
	fixtureModule(t)
	files := shippedStubs(t)
	if _, err := stubs.Load(files); err != nil {
		t.Fatalf("stubs.Load of the shipped files: %v", err)
	}
	old := stubs.Flag
	stubs.Flag = ""
	t.Cleanup(func() { stubs.Flag = old })
	t.Setenv(stubs.Env, strings.Join(files, ","))
	if got := stubs.Paths(); len(got) != 6 {
		t.Fatalf("EFFECTGO_STUBS names %d files, want 6", len(got))
	}
	if _, err := analyzeHTTP([]string{"./app", "./rowscan"}, effects.Analyzer, errset.Analyzer, ginhttp.Analyzer, huma.Analyzer); err != nil {
		t.Fatalf("analysis with the shipped stubs: %v", err)
	}
	if _, _, err := collectHTTP([]string{"./app"}, true); err != nil {
		t.Fatalf("effectgo http with the shipped stubs: %v", err)
	}
}

// TestHTTPUnionOfAdapters: effectgo http prints exactly the findings of the two
// adapters (the same load), sorted by severity, then position, and hides info
// findings unless -all is set.
func TestHTTPUnionOfAdapters(t *testing.T) {
	fixtureModule(t)
	useStubs(t, shippedStubs(t)...)

	union, _, err := collectHTTP([]string{"./..."}, true)
	if err != nil {
		t.Fatal(err)
	}
	gins, gRun, err := collectGin([]string{"./..."})
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, f := range gins {
		hf := relativize(classify(f, gRun), mustGetwd())
		want = append(want, key(hf))
	}
	hres, err := analyzeHuma([]string{"./..."})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range huma.Classify(hres).Findings {
		want = append(want, key(httpFinding(f)))
	}
	var got []string
	for _, f := range union {
		got = append(got, key(f))
	}
	if len(want) == 0 {
		t.Fatal("the fixture has no findings: the union check would be empty")
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("union differs from the adapters:\ngot  %d findings\nwant %d findings\n%s", len(got), len(want), strings.Join(got, "\n"))
	}

	// Order: severity rank, then position.
	for i := 1; i < len(union); i++ {
		a, b := union[i-1], union[i]
		if rankOf(a.Severity) > rankOf(b.Severity) || (rankOf(a.Severity) == rankOf(b.Severity) && a.Pos > b.Pos) {
			t.Errorf("not sorted by severity then pos at %d: %s %s before %s %s", i, a.Severity, a.Pos, b.Severity, b.Pos)
		}
	}
	// Info findings are hidden without -all.
	shown, _, err := collectHTTP([]string{"./..."}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range shown {
		if f.Severity == "info" {
			t.Errorf("info finding printed without -all: %+v", f)
		}
	}
}

func key(f httpFinding) string {
	return strings.Join([]string{f.Severity, f.Framework, f.Route, f.Handler, f.Pos, f.Variant, strconv.Itoa(f.Status), f.Via}, "|")
}

func TestRankOf(t *testing.T) {
	order := []string{"high", "medium", "leak", "info"}
	for i := 1; i < len(order); i++ {
		if rankOf(order[i-1]) >= rankOf(order[i]) {
			t.Errorf("%s must rank before %s", order[i-1], order[i])
		}
	}
	if rankOf("bogus") <= rankOf("info") {
		t.Errorf("an unknown severity must rank last")
	}
}
