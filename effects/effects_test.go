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

// TestMain clears EFFECTGO_STUBS and selects an explicit empty stub file: the
// tests state their stub files themselves (TestStubsEnvFallback sets the
// variable for its own run). Without -stubs, the zero configuration would load
// the shipped stdlib.stubs, which changes the expected results, e.g. (error).Error
// is pure.
func TestMain(m *testing.M) {
	os.Unsetenv(stubs.Env)
	none, err := filepath.Abs("testdata/stubs/none.stubs")
	if err != nil {
		panic(err)
	}
	stubs.Flag = none
	os.Exit(m.Run())
}

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

// TestTypeContractWithoutMethods: a type-level contract on an interface that declares
// no method of its own checks nothing, and is reported. One that declares a method is
// silent.
func TestTypeContractWithoutMethods(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "emptyiface")
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

// TestClosureAttribution covers func literals: a literal is charged to the
// declared function only when that function runs it (testdata/src/closure).
func TestClosureAttribution(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "closure")
}

// TestFuncValueStubs covers stub rules for a named func type (applies to every
// call through the type) and for a package-level func variable.
func TestFuncValueStubs(t *testing.T) {
	setStubs(t, absPath(t, "testdata/stubs/funcvals.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "funcvals")
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

// TestWitnessFlag: -witness prints "witness of <F>: <effect> via <chain>" for
// each effect; the chain starts below the function itself.
func TestWitnessFlag(t *testing.T) {
	old := effects.Analyzer.Flags.Lookup("witness").Value.String()
	if err := effects.Analyzer.Flags.Set("witness", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = effects.Analyzer.Flags.Set("witness", old) })
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "witness")
}

// TestInteractWitness: witness chains where a literal reaches the effect through
// parameter polymorphism, and where a stub on a named func type or on a func
// variable supplies the effect.
func TestInteractWitness(t *testing.T) {
	setStubs(t, absPath(t, "testdata/stubs/interact.stubs"))
	setFlag(t, "witness", "true")
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "interact")
}

// TestAllShippedStubFilesViaEnv: TestMain clears EFFECTGO_STUBS, so this sets it
// for its own run to the four shipped files together. Loading must succeed (no
// malformed line, no duplicate target across files).
func TestAllShippedStubFilesViaEnv(t *testing.T) {
	setStubs(t, "")
	t.Setenv("EFFECTGO_STUBS", strings.Join([]string{
		absPath(t, "../stubs/stdlib.stubs"),
		absPath(t, "../stubs/pgx.stubs"),
		absPath(t, "../stubs/gin.stubs"),
		absPath(t, "../stubs/huma.stubs"),
	}, ","))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "shipped")
}

// TestShippedHTTPRules checks the net/http rules of stubs/stdlib.stubs
// (testdata/src/httpw).
func TestShippedHTTPRules(t *testing.T) {
	setStubs(t, absPath(t, "../stubs/stdlib.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "httpw")
}

// TestPromotedInterfaceMethods checks the methods that a type gets from an embedded
// interface, against a contracted interface (testdata/src/promoted, promotedtimed,
// promotedlib): the net/http stub, an uncontracted interface (unknown), a contract
// of the embedded interface, a type's own method, and the methods promoted from
// embedded structs.
func TestPromotedInterfaceMethods(t *testing.T) {
	setStubs(t, absPath(t, "../stubs/stdlib.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "promoted", "promotedtimed")
}

// TestSuppressed covers the root rule (testdata/src/suppress and suppressdep):
// a violation whose only source is a violating callee is not reported, and the
// other cases (no contract, an allowing contract, an interface, a stub, a
// direct source, a second callee that does not violate) are reported.
func TestSuppressed(t *testing.T) {
	setStubs(t, absPath(t, "testdata/stubs/suppress.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "suppress")
}

// TestSuppressedAll: -all reports the callers too.
func TestSuppressedAll(t *testing.T) {
	setStubs(t, absPath(t, "testdata/stubs/suppress.stubs"))
	setFlag(t, "all", "true")
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "suppressall")
}
