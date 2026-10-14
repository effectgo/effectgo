package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The two silent misses that P10 fixed, end to end through the standalone run on
// scratch modules under testdata. A predicate whose constant false return comes
// before its errors.As test exports no test (predicate/early), so its caller is
// reported; the straight-line form still narrows (predicate/tail). A method that a
// type promotes from an embedded interface is checked against the contract of the
// interface that the type satisfies (promoted/statuswriter), and a type that defines
// its own method is checked on that method only (promoted/ownwriter).

// TestStandalonePredicateEarlyFalse is the caller of an early-false predicate.
func TestStandalonePredicateEarlyFalse(t *testing.T) {
	bin := buildEffectgo(t)
	t.Run("early", func(t *testing.T) {
		checkStandalone(t, bin, absTestdata(t, "predicate", "early"), 3, "1",
			"CallerBusy may return predearly.BusyError (via source), not in its //effectgo:errors contract")
	})
	t.Run("tail", func(t *testing.T) {
		checkStandalone(t, bin, absTestdata(t, "predicate", "tail"), 0, "0")
	})
}

// TestStandalonePromotedInterface is a method promoted from an embedded interface.
func TestStandalonePromotedInterface(t *testing.T) {
	bin := buildEffectgo(t)
	t.Run("statuswriter", func(t *testing.T) {
		checkStandalone(t, bin, absTestdata(t, "promoted", "statuswriter"), 3, "1",
			"(*statusWriter).Write violates contract of (Writer).Write [allow stdio]: has effect net via (*statusWriter).Write [promoted from embedded http.ResponseWriter] → http.(ResponseWriter).Write [stub]")
	})
	t.Run("ownwriter", func(t *testing.T) {
		checkStandalone(t, bin, absTestdata(t, "promoted", "ownwriter"), 0, "0")
	})
}

// absTestdata returns the absolute path of a scratch module under testdata.
func absTestdata(t *testing.T, parts ...string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join(append([]string{"testdata"}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// checkStandalone runs the standalone binary on dir and checks its exit code, that
// stdout is empty, the summary line with the number of findings, and that each
// expected diagnostic text occurs exactly once.
func checkStandalone(t *testing.T, bin, dir string, code int, findings string, diagnostics ...string) {
	t.Helper()
	stdout, stderr, got := runIn(t, bin, dir, "./...")
	if stdout != "" || got != code {
		t.Fatalf("standalone run: exit %d, stdout %q, want exit %d and no stdout\n%s", got, stdout, code, stderr)
	}
	summary := regexp.MustCompile(`^effectgo: \d+ packages? analysed \(\d+ test variants?\), ` + findings + ` findings?$`)
	if !summary.MatchString(lastLine(stderr)) {
		t.Errorf("last line %q, want the %s-findings summary\n%s", lastLine(stderr), findings, stderr)
	}
	for _, d := range diagnostics {
		if n := strings.Count(stderr, d); n != 1 {
			t.Errorf("diagnostic %q occurs %d times, want once\n%s", d, n, stderr)
		}
	}
}
