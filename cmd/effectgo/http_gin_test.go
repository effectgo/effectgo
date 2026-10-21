package main

import (
	"encoding/json"
	"strings"
	"testing"

	ginhttp "github.com/effectgo/effectgo/adapters/gin"
)

// run is a fixed aggregate: ErrA is mapped to 404 somewhere, ErrB is not.
func testRun() *ginRun {
	return &ginRun{
		mapped:  map[string]bool{"pkg.ErrA": true},
		routeOf: map[string]string{"/h.go:10": "GET /items/:id"},
	}
}

func TestClassifySeverities(t *testing.T) {
	run := testRun()
	cases := []struct {
		name string
		f    ginhttp.Finding
		want string
	}{
		{"mapped elsewhere", ginhttp.Finding{Kind: "reach", ErrKind: "sentinel", Key: "pkg.ErrA", Variant: "pkg.ErrA", Status: 500, Project: true}, "high"},
		{"never mapped", ginhttp.Finding{Kind: "reach", ErrKind: "sentinel", Key: "pkg.ErrB", Variant: "pkg.ErrB", Status: 500, Project: true}, "medium"},
		{"typed, never mapped", ginhttp.Finding{Kind: "reach", ErrKind: "type", Key: "pkg.ErrB", Variant: "pkg.ErrB", Status: 502}, "medium"},
		{"fresh", ginhttp.Finding{Kind: "reach", ErrKind: "fresh", Variant: "new@pkg.F", Status: 500}, "info"},
		{"opaque", ginhttp.Finding{Kind: "reach", ErrKind: "opaque", Variant: "?", Status: 500}, "info"},
		{"leak 500", ginhttp.Finding{Kind: "leak", ErrKind: "leak", Variant: "err", Status: 500}, "leak"},
	}
	for _, c := range cases {
		if got := classify(c.f, run).Severity; got != c.want {
			t.Errorf("%s: severity %q, want %q", c.name, got, c.want)
		}
	}
}

func TestJSONSchemaKeys(t *testing.T) {
	run := testRun()
	hf := classify(ginhttp.Finding{
		Kind: "reach", ErrKind: "sentinel", Key: "pkg.ErrA", Variant: "pkg.ErrA", Status: 500,
		Handler: "func literal in Get (/h.go:10)", HandlerPos: "/h.go:10", Pos: "/h.go:12", Via: "x",
	}, run)
	data, err := json.Marshal(hf)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"severity", "framework", "route", "handler", "pos", "variant", "status", "via", "reason"}
	if len(m) != len(want) {
		t.Errorf("record has %d keys, want exactly %d: %s", len(m), len(want), data)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %s", k, data)
		}
	}
	if m["framework"] != "gin" || m["status"] != float64(500) || m["route"] != "GET /items/:id" {
		t.Errorf("unexpected record %s", data)
	}
	if strings.Contains(string(data), "\\u0026") {
		t.Errorf("HTML escaping in %s", data)
	}
}
