package stubs

import (
	"path/filepath"
	"strings"
	"testing"
)

// Targets for named func types (pkg.T) and package-level func variables
// (pkg.Var) parse next to the function and method forms.
func TestParseFuncValueTargets(t *testing.T) {
	rules, err := Parse("x.stubs", []byte(`
effects (context.CancelFunc) pure
effects github.com/danielgtaylor/huma/v2.NewError pure
effects (context.Context).Err pure
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
		t.Fatalf("want 3 rules, got %d", len(rules))
	}
	ct := rules[0].Target
	if !ct.TypeOnly || ct.Method || ct.Pkg != "context" || ct.Type != "CancelFunc" {
		t.Errorf("named func type parsed wrong: %+v", ct)
	}
	if got := ct.String(); got != "(context.CancelFunc)" {
		t.Errorf("String of named func type = %q", got)
	}
	v := rules[1].Target
	if v.TypeOnly || v.Method || v.Pkg != "github.com/danielgtaylor/huma/v2" || v.Name != "NewError" {
		t.Errorf("package variable parsed wrong: %+v", v)
	}
	if got := v.String(); got != "github.com/danielgtaylor/huma/v2.NewError" {
		t.Errorf("String of package variable = %q", got)
	}
	if m := rules[2].Target; !m.Method || m.TypeOnly {
		t.Errorf("method target changed by the type form: %+v", m)
	}
}

// The type-only form is rejected where a type is not a func type, and for routes.
func TestParseFuncValueTargetErrors(t *testing.T) {
	cases := []struct{ text, want string }{
		{"effects (*a.T) pure\n", "x.stubs:1: target"},
		{"effects (error) pure\n", "x.stubs:1: target"},
		{"effects (a.T)x pure\n", "x.stubs:1: target"},
		{"route (a.T) GET 0 1\n", "x.stubs:1: route target"},
	}
	for _, c := range cases {
		_, err := Parse("x.stubs", []byte(c.text))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Parse(%q): error %v, want it to contain %q", c.text, err, c.want)
		}
	}
}

// Every stub file that ships in stubs/ parses. A malformed shipped line would
// otherwise surface only when a user loads that file.
func TestShippedStubFilesParse(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "stubs", "*.stubs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no shipped stub files found")
	}
	for _, f := range files {
		if _, err := ParseFile(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestParseSinkRules(t *testing.T) {
	rules, err := Parse("t.stubs", []byte("sink (*github.com/gin-gonic/gin.Context).JSON status=0 body=1\nsink fixture/httpapi.Detail status=1 body=2\nsink (*github.com/gin-gonic/gin.Context).AbortWithStatus status=0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 || rules[0].Verb != VerbSink || rules[1].Target.Name != "Detail" {
		t.Fatalf("unexpected rules %+v", rules)
	}
	if rules[0].Args[0] != "0" || rules[0].Args[1] != "1" || rules[2].Args[1] != "-1" {
		t.Errorf("argument indexes %v %v", rules[0].Args, rules[2].Args)
	}
	for _, bad := range []string{
		"sink fixture/httpapi.Detail body=2\n",
		"sink fixture/httpapi.Detail status=x\n",
		"sink fixture/httpapi.Detail status=1 extra=2\n",
		"sink (fixture/httpapi.Detail) status=1\n",
	} {
		if _, err := Parse("bad.stubs", []byte(bad)); err == nil {
			t.Errorf("want an error for %q", bad)
		}
	}
}

// A field source target parses for effects rules, and its String is the stub
// form. Its effects are fixed: via and stores items are rejected.
func TestParseFieldSourceTargets(t *testing.T) {
	rules, err := Parse("x.stubs", []byte(`
effects field:(net/http.Request).Body net
effects field:(net/http.Response).Body net,fs
effects field:(a/b.T).F pure
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
		t.Fatalf("want 3 rules, got %d", len(rules))
	}
	f := rules[0].Target
	if !f.Field || f.Method || f.TypeOnly || f.Pkg != "net/http" || f.Type != "Request" || f.Name != "Body" {
		t.Errorf("field source parsed wrong: %+v", f)
	}
	if got := f.String(); got != "field:(net/http.Request).Body" {
		t.Errorf("String of field source = %q", got)
	}
}

// A field source is rejected with the file and line when its form is wrong, or
// when it is used by a verb other than effects, or with via or stores items.
func TestParseFieldSourceErrors(t *testing.T) {
	cases := []struct{ text, want string }{
		{"effects field:(net/http.Request) net\n", "x.stubs:1: target"},
		{"effects field:(net/http.Request). net\n", "x.stubs:1: target"},
		{"effects field:net/http.Request.Body net\n", "x.stubs:1: target"},
		{"effects field:(*net/http.Request).Body net\n", "x.stubs:1: target"},
		{"effects field:(error).Error net\n", "x.stubs:1: target"},
		{"effects field:(Request).Body net\n", "x.stubs:1: target"},
		{"effects field:(net/http.Request).Body via:0.Read\n", "x.stubs:1: field source"},
		{"effects field:(net/http.Request).Body stores:0\n", "x.stubs:1: field source"},
		{"errors field:(net/http.Request).Body none\n", "x.stubs:1: target"},
		{"status field:(net/http.Request).Body 400\n", "x.stubs:1: target"},
		{"\neffects field:(net/http.Request).Body net,,fs\n", "x.stubs:2: "},
	}
	for _, c := range cases {
		_, err := Parse("x.stubs", []byte(c.text))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Parse(%q): error %v, want it to contain %q", c.text, err, c.want)
		}
	}
}
