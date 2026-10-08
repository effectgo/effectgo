package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faintghost/effectgo/examples/todo/domain"
)

// step is one request in a case. wantBody, if non-empty, is compared as
// JSON. The raw body of every step is also compared between v1 and v2.
type step struct {
	method     string
	path       string // "%s" is replaced by "/v1" or "/v2"
	body       string
	wantStatus int
	wantBody   string
}

type tcase struct {
	name  string
	steps []step
}

const titleA = `{"title":"buy milk"}`

var cases = []tcase{
	{"create ok", []step{
		{"POST", "%s/todos", titleA, 201, `{"id":1,"title":"buy milk","done":false}`},
	}},
	{"create empty title", []step{
		{"POST", "%s/todos", `{"title":"   "}`, 400, ""},
	}},
	{"create title too long", []step{
		{"POST", "%s/todos", `{"title":"` + strings.Repeat("x", 101) + `"}`, 400, ""},
	}},
	{"create malformed json", []step{
		{"POST", "%s/todos", `{"title":`, 400, ""},
	}},
	{"get ok", []step{
		{"POST", "%s/todos", titleA, 201, ""},
		{"GET", "%s/todos/1", "", 200, `{"id":1,"title":"buy milk","done":false}`},
	}},
	{"get missing", []step{
		{"GET", "%s/todos/999", "", 404, ""},
	}},
	{"bad id", []step{
		{"GET", "%s/todos/abc", "", 400, ""},
	}},
	{"update ok", []step{
		{"POST", "%s/todos", titleA, 201, ""},
		{"PUT", "%s/todos/1", `{"done":true}`, 200, `{"id":1,"title":"buy milk","done":true}`},
	}},
	{"update missing", []step{
		{"PUT", "%s/todos/999", `{"done":true}`, 404, ""},
	}},
	{"update invalid title", []step{
		{"POST", "%s/todos", titleA, 201, ""},
		{"PUT", "%s/todos/1", `{"title":""}`, 400, ""},
		{"GET", "%s/todos/1", "", 200, `{"id":1,"title":"buy milk","done":false}`},
	}},
	{"update bad id", []step{
		{"PUT", "%s/todos/abc", `{"done":true}`, 400, ""},
	}},
	{"delete ok", []step{
		{"POST", "%s/todos", titleA, 201, ""},
		{"DELETE", "%s/todos/1", "", 200, ""},
		{"GET", "%s/todos/1", "", 404, ""},
	}},
	{"delete missing", []step{
		{"DELETE", "%s/todos/999", "", 404, ""},
	}},
	{"delete bad id", []step{
		{"DELETE", "%s/todos/abc", "", 400, ""},
	}},
	{"list", []step{
		{"POST", "%s/todos", `{"title":"a"}`, 201, ""},
		{"POST", "%s/todos", `{"title":"b"}`, 201, ""},
		{"GET", "%s/todos", "", 200, `[{"id":1,"title":"a","done":false},{"id":2,"title":"b","done":false}]`},
	}},
	{"list empty", []step{
		{"GET", "%s/todos", "", 200, `[]`},
	}},
}

// run executes every step of c against a fresh server with the given prefix
// and returns the (status, raw body) of each step.
func run(t *testing.T, prefix string, c tcase) [][2]string {
	t.Helper()
	h := NewServer(domain.NewService()).Mux
	var out [][2]string
	for _, st := range c.steps {
		req := httptest.NewRequest(st.method, fmt.Sprintf(st.path, prefix), strings.NewReader(st.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		out = append(out, [2]string{fmt.Sprint(rec.Code), strings.TrimSpace(rec.Body.String())})
	}
	return out
}

func TestV1V2V3Parity(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v1 := run(t, "/v1", c)
			v2 := run(t, "/v2", c)
			v3 := run(t, "/v3", c)
			for i, st := range c.steps {
				if v1[i][0] != v2[i][0] || v1[i][0] != v3[i][0] {
					t.Errorf("step %d %s %s: status v1=%s v2=%s v3=%s", i, st.method, st.path, v1[i][0], v2[i][0], v3[i][0])
				}
				if v1[i][1] != v2[i][1] || v1[i][1] != v3[i][1] {
					t.Errorf("step %d %s %s: body differs\n v1=%s\n v2=%s\n v3=%s", i, st.method, st.path, v1[i][1], v2[i][1], v3[i][1])
				}
				if fmt.Sprint(st.wantStatus) != v1[i][0] {
					t.Errorf("step %d %s %s: status=%s want %d (body %s)", i, st.method, st.path, v1[i][0], st.wantStatus, v1[i][1])
				}
				if st.wantBody != "" && !jsonEqual(t, st.wantBody, v1[i][1]) {
					t.Errorf("step %d %s %s: body=%s want %s", i, st.method, st.path, v1[i][1], st.wantBody)
				}
			}
		})
	}
}

func jsonEqual(t *testing.T, want, got string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal([]byte(want), &a); err != nil {
		t.Fatalf("bad want json: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &b); err != nil {
		return false
	}
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

// TestOpenAPIParity checks that the generated spec gives each v2 route the
// same request and response schema refs as its v1 twin.
func TestOpenAPIParity(t *testing.T) {
	spec := specOf(t)
	pairs := []struct{ v1, v2, method string }{
		{"/v1/todos", "/v2/todos", "post"},
		{"/v1/todos", "/v2/todos", "get"},
		{"/v1/todos/{id}", "/v2/todos/{id}", "get"},
		{"/v1/todos/{id}", "/v2/todos/{id}", "put"},
		{"/v1/todos/{id}", "/v2/todos/{id}", "delete"},
	}
	for _, p := range pairs {
		a := opSummary(t, spec, p.v1, p.method)
		b := opSummary(t, spec, p.v2, p.method)
		c := opSummary(t, spec, strings.Replace(p.v1, "/v1/", "/v3/", 1), p.method)
		t.Logf("%s %s\n  v1: %s\n  v2: %s\n  v3: %s", strings.ToUpper(p.method), p.v1, a, b, c)
		if a != b || a != c {
			t.Errorf("%s %s: schema summary differs across v1/v2/v3", strings.ToUpper(p.method), p.v1)
		}
	}
}

func specOf(t *testing.T) map[string]any {
	t.Helper()
	s := NewServer(domain.NewService())
	raw, err := json.Marshal(s.OutputOpenAPISpec())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// opSummary renders the request-body schema ref and each response's schema
// ref for one operation, as a stable string.
func opSummary(t *testing.T, spec map[string]any, path, method string) string {
	t.Helper()
	op, ok := spec["paths"].(map[string]any)[path].(map[string]any)[method].(map[string]any)
	if !ok {
		t.Fatalf("no operation %s %s in spec", method, path)
	}
	var parts []string
	if rb, ok := op["requestBody"].(map[string]any); ok {
		parts = append(parts, "req="+schemaRef(rb))
	}
	codes := []string{}
	if resp, ok := op["responses"].(map[string]any); ok {
		for code := range resp {
			codes = append(codes, code)
		}
	}
	sortStrings(codes)
	for _, code := range codes {
		r := op["responses"].(map[string]any)[code].(map[string]any)
		parts = append(parts, code+"="+schemaRef(r))
	}
	return strings.Join(parts, " ")
}

func schemaRef(m map[string]any) string {
	content, _ := m["content"].(map[string]any)
	cts := make([]string, 0, len(content))
	for ct := range content {
		cts = append(cts, ct)
	}
	sortStrings(cts) // map order is random; sort for a stable summary
	for _, ct := range cts {
		sch, _ := content[ct].(map[string]any)["schema"].(map[string]any)
		if ref, ok := sch["$ref"].(string); ok {
			return ct + ":" + ref
		}
		if ty, ok := sch["type"].(string); ok {
			return ct + ":" + ty
		}
		return ct + ":?"
	}
	if d, ok := m["description"].(string); ok {
		return "(no content) " + d
	}
	return "(none)"
}

func sortStrings(s []string) {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// ---- benchmarks: full handler path through httptest ----

func benchServer(b *testing.B) (http.Handler, *domain.Service) {
	b.Helper()
	svc := domain.NewService()
	return NewServer(svc).Mux, svc
}

func benchGet(b *testing.B, prefix string) {
	h, svc := benchServer(b)
	if _, err := svc.Create("bench"); err != nil {
		b.Fatal(err)
	}
	url := prefix + "/todos/1"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

func benchCreate(b *testing.B, prefix string) {
	h, _ := benchServer(b)
	body := []byte(titleA)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, prefix+"/todos", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

func BenchmarkV1Get(b *testing.B)    { benchGet(b, "/v1") }
func BenchmarkV2Get(b *testing.B)    { benchGet(b, "/v2") }
func BenchmarkV1Create(b *testing.B) { benchCreate(b, "/v1") }
func BenchmarkV2Create(b *testing.B) { benchCreate(b, "/v2") }
