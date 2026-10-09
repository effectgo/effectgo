package huma

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/faintghost/effectgo/errset"
)

// The classification of variants that reach 500, on synthetic results. The
// mapping evidence is in one package and the route in another: a finding must
// see the mapping of the whole project, not only of its own package.
func TestClassifySeverities(t *testing.T) {
	const mod = "example.com/shop"
	routes := &Result{
		Module: mod,
		Routes: []Route{{
			Method: "GET", Path: "/orders/{id}", Pos: "api/orders.go:10:3",
			Handler: "func literal", HandlerPos: "api/orders.go:11:20",
			Variants: []RouteVariant{
				{Display: "store.ErrNotFound", Key: mod + "/store.ErrNotFound", Kind: errset.KindSentinel, Status: 500, Known: true, Via: []string{"Store.Get"}},
				{Display: "store.ErrClosed", Key: mod + "/store.ErrClosed", Kind: errset.KindSentinel, Status: 500, Known: true},
				{Display: "pgx.ErrNoRows", Key: "github.com/jackc/pgx/v5.ErrNoRows", Kind: errset.KindSentinel, Status: 500, Known: true},
				{Display: "?", Key: "?", Kind: errset.KindOpaque, Status: 500, Known: true, Via: []string{"parseID"}},
				{Display: "new@store.Get", Key: "new:" + mod + "/store.Get", Kind: errset.KindFresh, Status: 500, Known: true},
				{Display: "huma.Error404NotFound", Key: "github.com/danielgtaylor/huma/v2.Error404NotFound", Kind: errset.KindType, Status: 404, Known: true, Carried: true},
				{Display: "huma.Error500InternalServerError", Key: "github.com/danielgtaylor/huma/v2.Error500InternalServerError", Kind: errset.KindType, Status: 500, Known: true, Carried: true},
				{Display: "store.Unknown", Key: mod + "/store.Unknown", Kind: errset.KindType, Status: 0, Known: false},
			},
		}},
	}
	mapper := &Result{
		Module:   mod,
		Mappings: []Mapping{{Key: mod + "/store.ErrNotFound", Status: 404, Mapper: "mapErr", Pos: "api/errors.go:5"}},
	}
	rep := Classify([]*Result{mapper, routes})

	got := map[string]string{}
	for _, f := range rep.Findings {
		got[f.Variant] = f.Severity
	}
	want := map[string]string{
		"store.ErrNotFound": SeverityHigh,   // mapped to 404 in another package
		"store.ErrClosed":   SeverityMedium, // a project error no mapper maps
		"pgx.ErrNoRows":     SeverityInfo,   // a foreign error
		"?":                 SeverityInfo,   // opaque
		"new@store.Get":     SeverityInfo,   // fresh
	}
	for k, sev := range want {
		if got[k] != sev {
			t.Errorf("%s: severity %q, want %q", k, got[k], sev)
		}
	}
	if len(got) != len(want) {
		t.Errorf("findings: got %v, want exactly %v", got, want)
	}
	if rep.Stats.Intended500 != 1 {
		t.Errorf("intended 500: got %d, want 1 (the carried Error500)", rep.Stats.Intended500)
	}
	if rep.Stats.DefaultUnknown != 1 {
		t.Errorf("default unknown: got %d, want 1", rep.Stats.DefaultUnknown)
	}
	for _, f := range rep.Findings {
		if f.Variant == "store.ErrNotFound" && !strings.Contains(f.Reason, "maps it to 404") {
			t.Errorf("high finding reason does not cite the mapping: %q", f.Reason)
		}
		if f.Variant == "store.ErrNotFound" && f.Via != "Store.Get" {
			t.Errorf("via: got %q, want Store.Get", f.Via)
		}
		if f.Variant == "?" && f.Via != "parseID" {
			t.Errorf("via of ?: got %q", f.Via)
		}
		if f.Variant == "store.ErrClosed" && f.Via != "(direct)" {
			t.Errorf("empty chain must read (direct): got %q", f.Via)
		}
		if f.Route != "GET /orders/{id}" || f.Pos != "api/orders.go:10" || f.Handler != "func literal (api/orders.go:11)" || f.Status != 500 {
			t.Errorf("row fields: %+v", f)
		}
	}
}

// A mapping to 500 or more, or to nothing, is not evidence of a status below 500.
func TestMappingEvidenceNeedsNon5xx(t *testing.T) {
	const mod = "example.com/shop"
	res := &Result{
		Module: mod,
		Routes: []Route{{Method: "GET", Path: "/x", Pos: "x.go:1:1", Handler: "func literal",
			Variants: []RouteVariant{{Display: "store.E", Key: mod + "/store.E", Kind: errset.KindSentinel, Status: 500, Known: true}}}},
		Mappings: []Mapping{{Key: mod + "/store.E", Status: 500, Mapper: "m"}, {Key: mod + "/store.E", Status: 0, Mapper: "n"}},
	}
	rep := Classify([]*Result{res})
	if len(rep.Findings) != 1 || rep.Findings[0].Severity != SeverityMedium {
		t.Fatalf("a 500 mapping is not evidence: got %+v", rep.Findings)
	}
}

// Unresolved routes are counted, never classified.
func TestUnresolvedIsCounted(t *testing.T) {
	res := &Result{Module: "m", Routes: []Route{{Method: "GET", Path: "", Pos: "a.go:1:1", Unresolved: "the path is not a constant string"}}}
	rep := Classify([]*Result{res})
	if rep.Stats.Routes != 1 || len(rep.Stats.Unresolved) != 1 || len(rep.Findings) != 0 {
		t.Fatalf("unresolved route: %+v", rep.Stats)
	}
}

// The JSON schema: exactly the nine keys, in order, one object per line.
func TestWriteJSONSchema(t *testing.T) {
	rep := Report{Findings: []Finding{
		{Severity: SeverityHigh, Framework: "huma", Route: "GET /a", Handler: "h (a.go:1)", Pos: "a.go:2", Variant: "m.E", Status: 500, Via: "(direct)", Reason: "r"},
		{Severity: SeverityInfo, Framework: "huma", Route: "GET /b", Handler: "h (b.go:1)", Pos: "b.go:2", Variant: "?", Status: 500, Via: "(direct)", Reason: "r"},
	}}
	var b bytes.Buffer
	if err := WriteJSON(&b, rep, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("info hidden without -all: got %d lines", len(lines))
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &obj); err != nil {
		t.Fatal(err)
	}
	if len(obj) != 9 {
		t.Errorf("keys: got %d, want 9: %v", len(obj), obj)
	}
	if !strings.HasPrefix(lines[0], `{"severity":"high","framework":"huma","route":"GET /a","handler":"h (a.go:1)","pos":"a.go:2","variant":"m.E","status":500,"via":"(direct)","reason":"r"}`) {
		t.Errorf("schema order: %s", lines[0])
	}
	b.Reset()
	_ = WriteJSON(&b, rep, true)
	if n := strings.Count(b.String(), "\n"); n != 2 {
		t.Errorf("with -all: %d lines, want 2", n)
	}
}

func TestLineOf(t *testing.T) {
	for in, want := range map[string]string{"a/b.go:12:4": "a/b.go:12", "a/b.go:12": "a/b.go:12", "x": "x"} {
		if got := lineOf(in); got != want {
			t.Errorf("lineOf(%q) = %q, want %q", in, got, want)
		}
	}
}
