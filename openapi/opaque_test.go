package openapi_test

import (
	"strings"
	"testing"

	"github.com/faintghost/effectgo/openapi"
)

// The "?" variant (an unclassified error) is 500 unless the errmap maps it, and
// the dynamic status stays unknown (default). See errset.ownStatus. The package
// opaque holds one route per case.

// A route whose only variant is "?": 500, complete, no default.
func TestOpaqueIs500(t *testing.T) {
	r := find(t, routesOfPkg(t, "opaque"), "PUT", "/opaque")
	if r.Undocumented != "" {
		t.Fatalf("undocumented: %s", r.Undocumented)
	}
	if got := status(r, "?"); got != 500 {
		t.Errorf("? status %d, want 500", got)
	}
	if d := defaultOf(r); len(d) != 0 {
		t.Errorf("no variant is unknown, but default has %v", d)
	}
	if !r.Complete {
		t.Errorf("a route with only a 500 is complete")
	}
}

// A dynamic status (a type with a non-constant StatusCode) is default, as before.
func TestDynamicIsDefault(t *testing.T) {
	r := find(t, routesOfPkg(t, "opaque"), "PUT", "/dynamic")
	if got := defaultOf(r); len(got) != 1 || got[0] != "opaque.DynamicError" {
		t.Errorf("default variants %v, want [opaque.DynamicError]", got)
	}
	if r.Complete {
		t.Errorf("a dynamic status is unknown: want not complete")
	}
}

// The coincidence case: the conflict is 409 through the errmap, and the
// unclassified error is 500. The 500 must be attributed to "?", not to the
// conflict, and it must not fall back to default.
func TestConflictAndOpaqueDocs(t *testing.T) {
	r := find(t, routesOfPkg(t, "opaque"), "PUT", "/conflict-and-opaque")
	if got := status(r, "domain.ErrConflict"); got != 409 {
		t.Errorf("ErrConflict status %d, want 409", got)
	}
	if got := status(r, "?"); got != 500 {
		t.Errorf("? status %d, want 500", got)
	}
	if d := defaultOf(r); len(d) != 0 {
		t.Errorf("default has %v, want none", d)
	}

	frag := openapi.Fragment(mustOps(t, []openapi.Route{r}))
	resp := responsesOf(t, frag, "/conflict-and-opaque", "put")
	five, ok := resp["500"].(map[string]any)
	if !ok {
		t.Fatalf("no 500 response: %v", resp)
	}
	desc, _ := five["description"].(string)
	for _, want := range []string{"unclassified error (via", "becomes 500 unless it carries its own status"} {
		if !strings.Contains(desc, want) {
			t.Errorf("500 description %q does not contain %q", desc, want)
		}
	}
	if !listsVariant(five, "?") {
		t.Errorf("500 response does not list the ? variant: %v", five)
	}
	if _, ok := resp["default"]; ok {
		t.Errorf("no default response is expected, got %v", resp["default"])
	}
	if five409, ok := resp["409"].(map[string]any); !ok || !listsVariant(five409, "domain.ErrConflict") {
		t.Errorf("409 response does not list domain.ErrConflict: %v", resp["409"])
	}
}

// A handler that mixes the three kinds: each lands in its own response.
func TestMixedKinds(t *testing.T) {
	r := find(t, routesOfPkg(t, "opaque"), "PUT", "/mixed")
	if got := status(r, "domain.ErrConflict"); got != 409 {
		t.Errorf("ErrConflict status %d, want 409", got)
	}
	if got := status(r, "?"); got != 500 {
		t.Errorf("? status %d, want 500", got)
	}
	if d := defaultOf(r); len(d) != 1 || d[0] != "opaque.DynamicError" {
		t.Errorf("default variants %v, want [opaque.DynamicError]", d)
	}
	if r.Complete {
		t.Errorf("the dynamic variant makes the route incomplete")
	}
}

func mustOps(t *testing.T, rs []openapi.Route) []*openapi.Op {
	t.Helper()
	ops, undocumented := openapi.Operations(rs)
	if len(undocumented) != 0 {
		t.Fatalf("undocumented: %+v", undocumented)
	}
	return ops
}

// responsesOf returns the responses of one operation of a fragment.
func responsesOf(t *testing.T, frag map[string]any, path, method string) map[string]any {
	t.Helper()
	paths := frag["paths"].(map[string]any)
	item := paths[path].(map[string]any)
	op := item[method].(map[string]any)
	return op["responses"].(map[string]any)
}

// listsVariant reports whether a response lists the named variant in x-effectgo-errors.
func listsVariant(resp map[string]any, variant string) bool {
	list, _ := resp[openapi.ExtErrors].([]any)
	for _, e := range list {
		if m, ok := e.(map[string]any); ok && m["variant"] == variant {
			return true
		}
	}
	return false
}
