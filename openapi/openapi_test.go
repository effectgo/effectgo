package openapi_test

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/effectgo/effectgo/internal/stubs"
	"github.com/effectgo/effectgo/openapi"
)

func testdata(t *testing.T) string {
	t.Helper()
	return analysistest.TestData()
}

// routesOf runs the analyzer on package api with the route stubs and returns
// its routes.
func routesOf(t *testing.T) openapi.Result {
	t.Helper()
	return routesOfPkg(t, "api")
}

// routesOfPkg runs the analyzer on one package with the route stubs.
func routesOfPkg(t *testing.T, pkg string) openapi.Result {
	t.Helper()
	old := stubs.Flag
	stubs.Flag = filepath.Join(testdata(t), "stubs", "routes.stubs")
	t.Cleanup(func() { stubs.Flag = old })

	results := analysistest.Run(t, testdata(t), openapi.Analyzer, pkg)
	if len(results) != 1 {
		t.Fatalf("want one result, got %d", len(results))
	}
	res, ok := results[0].Result.(openapi.Result)
	if !ok {
		t.Fatalf("result has type %T, want openapi.Result", results[0].Result)
	}
	return res
}

// defaultOf returns the variants of r that have an unknown status (the
// "default" response).
func defaultOf(r openapi.Route) []string {
	var out []string
	for _, e := range r.Errors {
		if e.Status == 0 {
			out = append(out, e.Variant)
		}
	}
	return out
}

// find returns the route with the given method and path.
func find(t *testing.T, rs openapi.Result, method, path string) openapi.Route {
	t.Helper()
	for _, r := range rs {
		if r.Method == method && r.Path == path {
			return r
		}
	}
	t.Fatalf("no route %s %s in %d routes", method, path, len(rs))
	return openapi.Route{}
}

// status returns the status of the error named variant in r, or -1 if absent.
func status(r openapi.Route, variant string) int {
	for _, e := range r.Errors {
		if e.Variant == variant {
			return e.Status
		}
	}
	return -1
}

func TestRouteDiscovery(t *testing.T) {
	rs := routesOf(t)
	if len(rs) != 8 {
		for _, r := range rs {
			t.Logf("%s %q %s undocumented=%q", r.Method, r.Path, r.Handler, r.Undocumented)
		}
		t.Fatalf("want 8 routes, got %d", len(rs))
	}
}

// A same-package constructor with an errmap: statuses come from the errmap.
func TestConstructorWithErrmap(t *testing.T) {
	r := find(t, routesOf(t), "PUT", "/items/{id}")
	if r.Undocumented != "" {
		t.Fatalf("documented route is undocumented: %s", r.Undocumented)
	}
	if r.Handler != "update(...)" || r.Errmap != "httpres.ToHTTPError" {
		t.Errorf("handler %q errmap %q", r.Handler, r.Errmap)
	}
	if !r.Complete {
		t.Errorf("want complete")
	}
	// ErrNotFound -> NotFound{} (constant 404); ErrConflict -> HTTPError{Status: 409}.
	if got := status(r, "domain.ErrNotFound"); got != 404 {
		t.Errorf("ErrNotFound status %d, want 404", got)
	}
	if got := status(r, "domain.ErrConflict"); got != 409 {
		t.Errorf("ErrConflict status %d, want 409", got)
	}
}

// A handler function with a plain error that the errmap does not map: 500.
func TestUnmappedPlainErrorIs500(t *testing.T) {
	r := find(t, routesOf(t), "GET", "/items")
	if got := status(r, "domain.ValidationErr"); got != 500 {
		t.Errorf("ValidationErr status %d, want 500", got)
	}
	if !r.Complete {
		t.Errorf("a plain error gives a known status: want complete")
	}
}

// A handler declared in another package: its set and errmap come from facts.
func TestCrossPackageConstructor(t *testing.T) {
	r := find(t, routesOf(t), "PUT", "/cross/{id}")
	if r.Undocumented != "" {
		t.Fatalf("cross-package route undocumented: %s", r.Undocumented)
	}
	if status(r, "domain.ErrNotFound") != 404 || status(r, "domain.ErrConflict") != 409 {
		t.Errorf("cross-package statuses wrong: %+v", r.Errors)
	}
}

// A handler function in another package, passed by identifier.
func TestCrossPackageFunction(t *testing.T) {
	r := find(t, routesOf(t), "GET", "/show")
	if r.Undocumented != "" || status(r, "domain.ErrNotFound") != 404 {
		t.Errorf("show: undocumented=%q errors=%+v", r.Undocumented, r.Errors)
	}
}

func TestUndocumentedReasons(t *testing.T) {
	rs := routesOf(t)
	cases := []struct {
		method, path, want string
	}{
		{"GET", "/lit", "no errmap: a func literal"},
		{"POST", "/wrapped", "does not return a func literal"},
		{"PUT", "/nodirective", "no errmap: the handler has no //effectgo:handler directive"},
	}
	for _, c := range cases {
		r := find(t, rs, c.method, c.path)
		if !strings.Contains(r.Undocumented, c.want) {
			t.Errorf("%s %s: undocumented %q, want it to contain %q", c.method, c.path, r.Undocumented, c.want)
		}
	}
	// The non-constant path is found through its handler, which is documented.
	var found bool
	for _, r := range rs {
		if r.Method == "DELETE" {
			found = true
			if r.Path != "" || !strings.Contains(r.Undocumented, "not a constant string") {
				t.Errorf("DELETE with a variable path: path %q undocumented %q", r.Path, r.Undocumented)
			}
		}
	}
	if !found {
		t.Errorf("no DELETE route found")
	}
}

// rt/web's route functions are generic, with the router as argument 0 and the
// handler func(context.Context, In) (Out, error) as argument 2. The route is
// found at the generic call site, and the method value passed as the handler
// carries its error set and errmap.
func TestRtShapedGenericRoutes(t *testing.T) {
	rs := routesOfPkg(t, "rtapi")
	if len(rs) != 2 {
		t.Fatalf("want 2 routes, got %d", len(rs))
	}
	get := find(t, rs, "GET", "/contacts/{id}")
	if get.Undocumented != "" || get.Handler != "Get" || !get.Complete {
		t.Fatalf("GET: handler %q undocumented=%q complete=%v", get.Handler, get.Undocumented, get.Complete)
	}
	if status(get, "domain.ErrNotFound") != 404 {
		t.Errorf("GET ErrNotFound status %d, want 404", status(get, "domain.ErrNotFound"))
	}
	post := find(t, rs, "POST", "/contact")
	if post.Undocumented != "" || post.Handler != "Update" || !post.Complete {
		t.Fatalf("POST: handler %q undocumented=%q complete=%v", post.Handler, post.Undocumented, post.Complete)
	}
	if status(post, "domain.ErrConflict") != 409 {
		t.Errorf("POST ErrConflict status %d, want 409", status(post, "domain.ErrConflict"))
	}
	if status(post, "domain.ValidationErr") != 500 {
		t.Errorf("POST ValidationErr status %d, want 500 (no status of its own)", status(post, "domain.ValidationErr"))
	}
}
