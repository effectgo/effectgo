package errset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStub(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.stubs")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A malformed stub line is an error that names the file and the line.
func TestMalformedStubIsError(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"unknown verb", "bogus a.B c\n", "x.stubs:1: unknown verb \"bogus\""},
		{"missing variants", "# c\nerrors a.F\n", "x.stubs:2: errors wants <target> <variants>"},
		{"unqualified variant", "errors a.F ErrX\n", "x.stubs:1: variant \"ErrX\""},
		{"param in stub", "errors a.F param:0\n", "x.stubs:1: param:0: param: variants are not allowed"},
		{"none in list", "errors a.F none,?\n", "x.stubs:1: none must be the whole variant list"},
		{"unqualified target", "errors F ?\n", "x.stubs:1: target"},
		{"bad method target", "errors (a.T) ?\n", "x.stubs:1: target"},
		{"empty variant", "errors a.F ?,\n", "x.stubs:1: variant"},
		{"effects shape", "effects a.F\n", "x.stubs:1: effects wants"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseStubFile(writeStub(t, c.text))
			if err == nil {
				t.Fatalf("no error for %q", c.text)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err, c.want)
			}
		})
	}
}

// Well-formed stub text parses into errors rules; effects rules are skipped.
func TestStubParsing(t *testing.T) {
	rules, err := parseStubFile(writeStub(t, `
# comment
effects a.F io,net   # ignored
pure-not-a-verb-here ignored
`))
	if err == nil {
		t.Fatalf("unknown verb accepted: %v", rules)
	}
	rules, err = parseStubFile(writeStub(t, `
# comment
effects a.F io,net   # ignored by errset
errors a.F pkg.ErrX,type:pkg.T,iface:pkg.I,?   # all four forms
errors (*pkg.T).M none
errors (pkg/sub.Iface).Method pkg.ErrY
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
		t.Fatalf("want 3 errors rules, got %d", len(rules))
	}
	kinds := []string{KindSentinel, KindType, KindIface, KindOpaque}
	for i, tok := range rules[0].tokens {
		if tok.kind != kinds[i] {
			t.Errorf("token %d: kind %s, want %s", i, tok.kind, kinds[i])
		}
	}
	if !rules[1].none || rules[1].target.Method != true || rules[1].target.Type != "T" {
		t.Errorf("method rule parsed wrong: %+v", rules[1])
	}
	if rules[2].target.Pkg != "pkg/sub" || rules[2].target.Type != "Iface" || rules[2].target.Name != "Method" {
		t.Errorf("interface target parsed wrong: %+v", rules[2].target)
	}
}
