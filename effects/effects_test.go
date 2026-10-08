package effects_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/faintghost/effectgo/effects"
	"github.com/faintghost/effectgo/internal/stubs"
)

func TestSamePackageTransitive(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "chain")
}

func TestCrossPackage(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "domain")
}

func TestRecursion(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "rec")
}

func TestGoPanicGlobalClosure(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "effs")
}

func TestInterfaceAndDynamicCalls(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "iface")
}

func TestAllowAndBadDirectives(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "allow")
}

func TestReportFlag(t *testing.T) {
	setFlag(t, "report", "true")
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "report")
}

func TestKnownLimits(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "limits")
}

// TestInterfaceContracts covers contracts on interface methods used across
// packages, conformance (violation and OK, imported and local interfaces),
// and a method-level directive overriding a type-level one.
func TestInterfaceContracts(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "ifacedef", "ifaceuse")
}

// TestPolymorphism covers placeholders for calls through func parameters,
// substituted at call sites within a package and across packages, including
// composition through two levels.
func TestPolymorphism(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "polylib", "poly")
}

// TestStubOverride covers a test stub file: it overrides the taxonomy, supplies
// effects for an interface call, and carries an errors line that is ignored.
func TestStubOverride(t *testing.T) {
	setStubs(t, absPath(t, "testdata/stubs/test.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "stubbed")
}

// TestShippedStubs checks the stubs/stdlib.stubs file shipped with the analyzer:
// error.Error and fmt.Stringer.String are pure, io.Copy is unknown.
func TestShippedStubs(t *testing.T) {
	setStubs(t, absPath(t, "../stubs/stdlib.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "shipped")
}

// TestStubsEnvFallback checks that EFFECTGO_STUBS is used when -stubs is empty.
func TestStubsEnvFallback(t *testing.T) {
	setStubs(t, "")
	t.Setenv("EFFECTGO_STUBS", absPath(t, "../stubs/stdlib.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "shipped")
}

// TestMalformedStubIsHardError builds the standalone checker and runs it with
// a malformed stub file. It must exit non-zero and name the file and line.
// Done as a process run because singlechecker is the place where a run error
// becomes a visible failure.
func TestMalformedStubIsHardError(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "effectgo")
	build := exec.Command("go", "build", "-o", bin, absPath(t, "../cmd/effectgo"))
	build.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building effectgo: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "-stubs="+absPath(t, "testdata/stubs/bad.stubs"), "./domain")
	cmd.Dir = absPath(t, "../examples/layers")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a non-zero exit for a malformed stub file; output:\n%s", out)
	}
	if !strings.Contains(string(out), "bad.stubs:4:") || !strings.Contains(string(out), `unknown effect name "tiem"`) {
		t.Fatalf("error does not name file:line and the bad effect; output:\n%s", out)
	}
}

// setStubs sets the shared -stubs value (internal/stubs.Flag) for the test.
func setStubs(t *testing.T, value string) {
	t.Helper()
	old := stubs.Flag
	stubs.Flag = value
	t.Cleanup(func() { stubs.Flag = old })
}

// setFlag sets an analyzer flag for the duration of the test.
func setFlag(t *testing.T, name, value string) {
	t.Helper()
	old := effects.Analyzer.Flags.Lookup(name).Value.String()
	if err := effects.Analyzer.Flags.Set(name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = effects.Analyzer.Flags.Set(name, old) })
}

func absPath(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
