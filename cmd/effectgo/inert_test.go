package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The inert-directive probe (testdata/inert/p.go), end to end. Each placement or
// spelling that no analyzer read is reported exactly once by the combined binary,
// on the standalone run and under go vet, and the four diagnostics that were
// printed before are unchanged. Line 32 (an enum with no constants) is not
// reported: it is not a placement, and no analyzer defines an empty enum as inert.

// probeWant maps each line of the probe that has a diagnostic to a fragment of it.
var probeWant = map[int]string{
	14: "effectgo:errors on NoErrResult checks nothing: it has no error result",
	19: "effectgo:handler on H checks nothing: it has no error result; put the directive on the inner function that returns error",
	27: "S: //effectgo:sealed applies to interfaces and to integer or string types",
	37: "effectgo:mustuse on NoResults checks nothing: it has no results",
	41: "//effectgo:pure is not attached to a function, interface or type declaration and is not read",
	46: "//effectgo:package is only valid in a package doc comment",
	51: "//effectgo:pure is not attached to a function, interface or type declaration and is not read",
	57: "// effectgo:pure is not a directive: remove the space after //",
	62: "//effectgo:pure is not attached to a function, interface or type declaration and is not read",
	69: "effectgo:errmap: BadMap must have signature func(error) error",
	73: "//effectgo:allow needs at least one effect name",
	78: "//effectgo:pure is not attached to a function, interface or type declaration and is not read",
}

// probeLine matches one diagnostic of the probe: "<path>/p.go:line:col: message".
var probeLine = regexp.MustCompile(`p\.go:(\d+):\d+: (.*)$`)

// TestInertDirectivesOneDiagnostic runs the standalone binary and go vet on the
// probe and checks that each probe line has exactly one diagnostic that says what
// the placement or spelling is wrong.
func TestInertDirectivesOneDiagnostic(t *testing.T) {
	bin := buildEffectgo(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "inert"))
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runIn(t, bin, dir, "./...")
	if stdout != "" || code != 3 {
		t.Fatalf("standalone run: exit %d, stdout %q, want exit 3 and no stdout\n%s", code, stdout, stderr)
	}
	checkProbe(t, "standalone", stderr)

	cmd := exec.Command("go", "vet", "-vettool="+bin, "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	var vetOut bytes.Buffer
	cmd.Stderr = &vetOut
	_ = cmd.Run()
	checkProbe(t, "go vet", vetOut.String())
}

// checkProbe counts the probe diagnostics in out and checks each against probeWant.
func checkProbe(t *testing.T, how, out string) {
	t.Helper()
	count := map[int]int{}
	for _, line := range strings.Split(out, "\n") {
		m := probeLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatal(err)
		}
		count[n]++
		want, ok := probeWant[n]
		if !ok {
			t.Errorf("%s: unexpected diagnostic on line %d: %s", how, n, m[2])
		} else if !strings.Contains(m[2], want) {
			t.Errorf("%s: line %d: %q does not contain %q", how, n, m[2], want)
		}
	}
	for n := range probeWant {
		if count[n] != 1 {
			t.Errorf("%s: line %d has %d diagnostics, want exactly one:\n%s", how, n, count[n], out)
		}
	}
}
