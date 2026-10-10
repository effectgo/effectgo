package stubs

// Interaction (b) of the P2 merge: one stub file holds every verb form of both
// HTTP branches (sink, status with a code, arg:N and a type target, route with
// indexes and @N.Field, errdefault, effects and errors), and a malformed line is
// an error that names its file and line.

import (
	"strings"
	"testing"
)

const allVerbs = `# every verb form of the gin and huma branches, in one file
effects github.com/x/y.F db
effects (github.com/x/y.T).M pure
errors github.com/x/y.F github.com/x/y.ErrA,?
errors (*github.com/x/y.T).M none
status (github.com/x/y.Code) 400
status github.com/x/y.Func 404
status github.com/x/y.NewErr arg:0
route github.com/x/y.Get GET 1 2
route github.com/x/y.Register @1.Method @1.Path 2
route github.com/x/y.Post POST @1.Path 2
errdefault github.com/x/y passthrough=iface:github.com/x/y.StatusError,fallback=500
sink github.com/x/y.JSON status=1 body=2
sink github.com/x/y.Abort status=1
`

func TestOneFileAllVerbs(t *testing.T) {
	rules, err := Parse("all.stubs", []byte(allVerbs))
	if err != nil {
		t.Fatalf("every verb form must parse: %v", err)
	}
	var verbs []string
	for _, r := range rules {
		verbs = append(verbs, r.Verb)
	}
	want := "effects effects errors errors status status status route route route errdefault sink sink"
	if got := strings.Join(verbs, " "); got != want {
		t.Fatalf("verbs = %q, want %q", got, want)
	}
	for i, r := range rules {
		if r.File != "all.stubs" || r.Line == 0 {
			t.Errorf("rule %d: file %q line %d, want the file and a line", i, r.File, r.Line)
		}
	}
	// The sink forms keep their argument indexes; a sink without body has -1.
	if a := rules[11].Args; len(a) != 2 || a[0] != "1" || a[1] != "2" {
		t.Errorf("sink with body: args %v", a)
	}
	if a := rules[12].Args; len(a) != 2 || a[0] != "1" || a[1] != "-1" {
		t.Errorf("sink without body: args %v", a)
	}
	// The route forms: a constant method, and @N.Field indexes.
	if rules[8].Value != "@1.Method" || rules[8].Args[0] != "@1.Path" {
		t.Errorf("route with @N.Field method and path: value %q args %v", rules[8].Value, rules[8].Args)
	}
	if rules[9].Value != "POST" || rules[9].Args[0] != "@1.Path" {
		t.Errorf("route with a constant method and @N.Field path: value %q args %v", rules[9].Value, rules[9].Args)
	}
}

// TestMalformedLinesNameFileAndLine: each bad line is rejected, and the error
// names the file and the line of the bad line (here line 2).
func TestMalformedLinesNameFileAndLine(t *testing.T) {
	bad := []string{
		"sink github.com/x/y.F body=2",               // no status
		"sink github.com/x/y.F status=x",             // not an index
		"sink github.com/x/y.F status=1 extra=3",     // unknown key
		"sink (github.com/x/y.T) status=1",           // a type is not a sink
		"status github.com/x/y.F arg:x",              // the status argument is not an index
		"route github.com/x/y.Get FETCH 1 2",         // unknown method
		"route github.com/x/y.Get GET 1",             // too few fields
		"route github.com/x/y.Get GET 1 two",         // handler index is not a number
		"route (github.com/x/y.T).M GET 1 2",         // a method is not a route target
		"errdefault github.com/x/y(z) passthrough=x", // a target with parentheses
		"frobnicate github.com/x/y.F pure",           // unknown verb
	}
	for _, line := range bad {
		data := "effects github.com/x/y.G pure\n" + line + "\n"
		_, err := Parse("bad.stubs", []byte(data))
		if err == nil {
			t.Errorf("%q: accepted, want an error", line)
			continue
		}
		if !strings.HasPrefix(err.Error(), "bad.stubs:2:") {
			t.Errorf("%q: error %q does not name bad.stubs:2", line, err)
		}
	}
}
