package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// formsWant is the diagnostic of each line of testdata/forms/forms.go that has one,
// located by a substring of the line. Every other line has none. The three forms that
// read a directive with a trailing comment or an argument are checked on the line of
// the directive, and the rest on the line that the analyzer reports.
var formsWant = []struct{ line, msg string }{
	{"//effectgo:mustuse extra", "//effectgo:mustuse takes no arguments"},
	{"MakeExtra() // discarded", "result of example.com/forms.MakeExtra is discarded"},
	{"MakeNoted() // discarded", "result of example.com/forms.MakeNoted is discarded"},
	{"//effectgo:mustuse//note", `unknown effectgo directive "mustuse//note" is not read here`},
	{"switch k {", "missing cases in switch on Kind: KB"},
	{"//effectgo:sealed extra", "//effectgo:sealed takes no arguments"},
	{"switch m {", "missing cases in switch on Mode: MB"},
	{"type Base interface", "effectgo:pure on Base checks nothing: an interface contract covers the methods it declares, and Base declares none"},
	{"type Embedded interface", "effectgo:errors on Embedded checks nothing: an interface contract covers the methods it declares, and Embedded declares none"},
	{"func Tabbed() int", "effectgo:mustuse on Tabbed checks nothing: //effectgo:mustuse marks a named type, not a function"},
}

// formsLine matches a diagnostic of the forms module: "<path>/forms.go:line:col: message".
var formsLine = regexp.MustCompile(`forms\.go:(\d+):\d+: (.*)$`)

// TestDirectiveFormsOneDiagnostic runs the standalone binary on testdata/forms, a module
// that uses the directive forms with a trailing comment, with an argument, glued to the
// comment, or on an interface that declares no methods. Each problem is reported exactly
// once, by the analyzer that reads the directive there, and nothing else is reported.
func TestDirectiveFormsOneDiagnostic(t *testing.T) {
	bin := buildEffectgo(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "forms"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(dir, "forms.go"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	want := map[int]string{}
	for _, w := range formsWant {
		n := lineContaining(t, lines, w.line)
		want[n] = w.msg
	}

	stdout, stderr, code := runIn(t, bin, dir, "./...")
	if stdout != "" || code != 3 {
		t.Fatalf("standalone run: exit %d, stdout %q, want exit 3 and no stdout\n%s", code, stdout, stderr)
	}
	count := map[int]int{}
	for _, l := range strings.Split(stderr, "\n") {
		m := formsLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatal(err)
		}
		count[n]++
		msg, ok := want[n]
		switch {
		case !ok:
			t.Errorf("unexpected diagnostic on line %d: %s", n, m[2])
		case !strings.Contains(m[2], msg):
			t.Errorf("line %d: %q does not contain %q", n, m[2], msg)
		}
	}
	for n := range want {
		if count[n] != 1 {
			t.Errorf("line %d has %d diagnostics, want exactly one:\n%s", n, count[n], stderr)
		}
	}
}

// lineContaining returns the number of the one line of lines that contains sub.
func lineContaining(t *testing.T, lines []string, sub string) int {
	t.Helper()
	found, n := 0, 0
	for i, l := range lines {
		if strings.Contains(l, sub) {
			found = i + 1
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%q matches %d lines of forms.go, want exactly one", sub, n)
	}
	return found
}
