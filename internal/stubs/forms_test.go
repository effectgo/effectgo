package stubs

import (
	"strings"
	"testing"
)

func TestStatusValueForms(t *testing.T) {
	v, err := ParseStatusValue("404")
	if err != nil || v.Code != 404 || v.ArgIdx != -1 {
		t.Fatalf("404: got %+v, %v", v, err)
	}
	v, err = ParseStatusValue("arg:0")
	if err != nil || v.ArgIdx != 0 || v.Code != 0 {
		t.Fatalf("arg:0: got %+v, %v", v, err)
	}
	for _, bad := range []string{"99", "600", "arg:-1", "arg:x", "four"} {
		if _, err := ParseStatusValue(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestFieldRefAndRouteArg(t *testing.T) {
	ref, ok, err := ParseFieldRef("@1.Path")
	if !ok || err != nil || ref.Arg != 1 || ref.Field != "Path" {
		t.Fatalf("@1.Path: got %+v ok=%v err=%v", ref, ok, err)
	}
	if _, ok, _ := ParseFieldRef("GET"); ok {
		t.Errorf("GET is not a field reference")
	}
	for _, bad := range []string{"@1", "@x.Path", "@1.2x"} {
		if _, ok, err := ParseFieldRef(bad); !ok || err == nil {
			t.Errorf("%q: want a malformed field reference (ok=%v err=%v)", bad, ok, err)
		}
	}
	a, err := ParseRouteArg("1")
	if err != nil || a.Index != 1 || a.Field != nil {
		t.Fatalf("1: got %+v %v", a, err)
	}
	a, err = ParseRouteArg("@1.Path")
	if err != nil || a.Field == nil || a.Field.Field != "Path" {
		t.Fatalf("@1.Path: got %+v %v", a, err)
	}
}

func TestErrdefaultPolicy(t *testing.T) {
	ed, err := ParseErrdefault("passthrough=iface:github.com/danielgtaylor/huma/v2.StatusError,fallback=500")
	if err != nil {
		t.Fatal(err)
	}
	if ed.Iface != "github.com/danielgtaylor/huma/v2.StatusError" || ed.Fallback != 500 {
		t.Fatalf("got %+v", ed)
	}
	for _, bad := range []string{
		"fallback=500",
		"passthrough=iface:x.I",
		"passthrough=type:x.T,fallback=500",
		"passthrough=iface:x.I,fallback=42",
		"passthrough=iface:x.I,fallback=500,extra=1",
		"passthrough=iface:nodot,fallback=500",
	} {
		if _, err := ParseErrdefault(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

// The new verb forms parse in a stub file, and malformed ones name the line.
func TestParseNewForms(t *testing.T) {
	good := strings.Join([]string{
		"status github.com/x/huma.Error404NotFound 404",
		"status github.com/x/huma.NewError arg:0",
		"status (github.com/x/huma.Type) 409",
		"route github.com/x/huma.Register @1.Method @1.Path 2",
		"route github.com/x/huma.Get GET 1 2",
		"errdefault github.com/x/huma passthrough=iface:github.com/x/huma.StatusError,fallback=500",
	}, "\n")
	rules, err := Parse("good.stubs", []byte(good))
	if err != nil {
		t.Fatalf("good forms: %v", err)
	}
	if len(rules) != 6 {
		t.Fatalf("rules: got %d, want 6", len(rules))
	}
	if rules[0].Target.Name != "Error404NotFound" || rules[0].Target.Type != "" {
		t.Errorf("function status target: got %+v", rules[0].Target)
	}
	if rules[2].Target.Type != "Type" || rules[2].Target.Name != "" {
		t.Errorf("type status target: got %+v", rules[2].Target)
	}
	if rules[3].Value != "@1.Method" || rules[3].Args[0] != "@1.Path" {
		t.Errorf("register route: got value %q args %v", rules[3].Value, rules[3].Args)
	}
	if rules[5].Target.Pkg != "github.com/x/huma" {
		t.Errorf("errdefault target: got %+v", rules[5].Target)
	}

	for _, bad := range []string{
		"status github.com/x/huma.F 99",
		"status github.com/x/huma.F arg:x",
		"route github.com/x/huma.F PUTT 1 2",
		"route github.com/x/huma.F GET @1 2",
		"route github.com/x/huma.F GET 1 x",
		"errdefault github.com/x/huma (x) passthrough=iface:a.I,fallback=500",
		"errdefault github.com/x/huma passthrough=iface:a.I",
	} {
		if _, err := Parse("bad.stubs", []byte(bad)); err == nil {
			t.Errorf("%q: want a parse error", bad)
		} else if !strings.Contains(err.Error(), "bad.stubs:1") {
			t.Errorf("%q: error does not name the line: %v", bad, err)
		}
	}
}
