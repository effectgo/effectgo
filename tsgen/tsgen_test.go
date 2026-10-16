package tsgen

import (
	"path/filepath"
	"strings"
	"testing"
)

// generate collects the routes of the patterns in dir and generates the file.
func generate(t *testing.T, dir string, patterns ...string) (string, error) {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Collect(abs, patterns...)
	if err != nil {
		t.Fatalf("collect %v in %s: %v", patterns, dir, err)
	}
	return Generate(c.Results, "api", c.Module, c.Packages)
}

// wantErr asserts that generation fails and that the error contains each substring.
func wantErr(t *testing.T, err error, subs ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no generation error; want one containing %q", subs)
	}
	for _, s := range subs {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not contain %q", err.Error(), s)
		}
	}
}

// TestProblemIsGenerationError: a field type that rt refuses stops the generator,
// with the position of the field.
func TestProblemIsGenerationError(t *testing.T) {
	_, err := generate(t, "testdata/mod", "./bad/problem")
	wantErr(t, err, "problem.go:", "field In.Ch: type chan int is not supported")
}

// TestMultipleCookiesUseBrowserCredentials: cookie fields come from the browser
// cookie jar, so multiple names are supported without generating Cookie headers.
func TestMultipleCookiesUseBrowserCredentials(t *testing.T) {
	out, err := generate(t, "testdata/mod", "./bad/cookies")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "headers: Schema.Struct") || strings.Contains(out, "Cookie: Schema") {
		t.Fatalf("generated client exposes a Cookie header:\n%s", out)
	}
	for _, want := range []string{"BrowserFetchHttpClient", `credentials: "include"`, `HttpApiEndpoint.get("get"`} {
		if !strings.Contains(out, want) {
			t.Errorf("generated output lacks %q:\n%s", want, out)
		}
	}
}

// TestRecursiveIsGenerationError: a type that refers to itself is not supported.
func TestRecursiveIsGenerationError(t *testing.T) {
	_, err := generate(t, "testdata/mod", "./bad/recursion")
	wantErr(t, err, "recursive type Node is not supported", "recursion.go:")
}

// TestNameConflictIsGenerationError: two types with one name, from two packages,
// stop the generator and name both.
func TestNameConflictIsGenerationError(t *testing.T) {
	_, err := generate(t, "testdata/mod", "./bad/conflict")
	wantErr(t, err, "two types are named Item", "example.com/tsbad/bad/conflict/a.Item", "example.com/tsbad/bad/conflict/b.Item")
}

// TestErrorListOrder: the handled variants are sorted by status, then by name;
// the framework's errors follow in their fixed order.
func TestErrorListOrder(t *testing.T) {
	out, err := generate(t, "testdata/mod", "./ok/order")
	if err != nil {
		t.Fatal(err)
	}
	want := "error: [Bad, ANotFound, ZNotFound, DecodeError, InternalError],"
	if !strings.Contains(out, want) {
		t.Errorf("no %q in:\n%s", want, out)
	}
}

// TestExampleWebErrorList: updateContact is a POST, so it lists BodyTooLarge; its
// 400 ValidationError comes before the 404 ContactNotFound.
func TestExampleWebErrorList(t *testing.T) {
	out, err := generate(t, "../examples/web", "./...")
	if err != nil {
		t.Fatal(err)
	}
	want := "error: [ValidationError, ContactNotFound, DecodeError, BodyTooLarge, InternalError],"
	if !strings.Contains(out, want) {
		t.Errorf("no %q in:\n%s", want, out)
	}
}

// TestDeterministic: two runs write the same bytes, for examples/web and for the
// golden packages.
func TestDeterministic(t *testing.T) {
	for _, in := range []struct {
		dir      string
		patterns []string
	}{
		{"../examples/web", []string{"./..."}},
		{"testdata/mod", goldenPatterns},
	} {
		a, err := generate(t, in.dir, in.patterns...)
		if err != nil {
			t.Fatal(err)
		}
		b, err := generate(t, in.dir, in.patterns...)
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Errorf("two runs differ for %s", in.dir)
		}
	}
}

// TestRTNameConflict: a user type named like one rt/web or rt/schema writes is a
// generation error that names both types, for each of the framework names.
func TestRTNameConflict(t *testing.T) {
	_, err := generate(t, "testdata/mod", "./bad/rtname")
	wantErr(t, err,
		"two types are named InternalError", "example.com/tsbad/bad/rtname.InternalError", "github.com/faintghost/effectgo/rt/web.InternalError",
		"two types are named DecodeError", "example.com/tsbad/bad/rtname.DecodeError", "github.com/faintghost/effectgo/rt/schema.DecodeError",
		"two types are named BodyTooLarge", "example.com/tsbad/bad/rtname.BodyTooLarge", "github.com/faintghost/effectgo/rt/web.BodyTooLarge",
		"two types are named Issue", "example.com/tsbad/bad/rtname.Issue", "github.com/faintghost/effectgo/rt/schema.Issue")
}

// TestErrorListNoRepeat: a handler that returns the framework's InternalError lists
// it once, in its own place (by status), and the framework's list does not add it again.
func TestErrorListNoRepeat(t *testing.T) {
	out, err := generate(t, "testdata/mod", "./ok/repeat")
	if err != nil {
		t.Fatal(err)
	}
	want := "error: [InternalError, DecodeError],"
	if !strings.Contains(out, want) {
		t.Errorf("no %q in:\n%s", want, out)
	}
}

// TestLowerCamel: the group and endpoint names lower the leading capitals as Go
// writes an acronym in a lower-case name.
func TestLowerCamel(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"Store", "store"},
		{"API", "api"},
		{"ID", "id"},
		{"A", "a"},
		{"HTTPServer", "httpServer"},
		{"URLMap", "urlMap"},
		{"ABc", "aBc"},
		{"UpdateContact", "updateContact"},
		{"updateContact", "updateContact"},
		{"V2API", "v2API"}, // a digit ends the leading run of capitals
		{"IDs", "ids"},
		{"IDsByName", "idsByName"},
		{"URLsV2", "urlsV2"},
		{"Issues", "issues"}, // an s that is not single is not a plural acronym
	} {
		if got := lowerCamel(c.in); got != c.want {
			t.Errorf("lowerCamel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
