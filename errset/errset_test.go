package errset_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
	"golang.org/x/tools/go/analysis/analysistest"
)

// TestMain clears EFFECTGO_STUBS and selects an explicit empty stub file: the
// tests state their stub files themselves. The shipped stdlib.stubs would change
// the expected results (it makes (error).Error pure and gives strconv.Atoi a
// *strconv.NumError), and without -stubs the zero configuration would load it,
// so an inherited value must not reach them.
func TestMain(m *testing.M) {
	os.Unsetenv(stubs.Env)
	none, err := filepath.Abs("testdata/stubs/none.stubs")
	if err != nil {
		panic(err)
	}
	stubs.Flag = none
	// EFFECTGO_TEST_ENGINE=ssa runs the whole suite with the SSA engine.
	if e := os.Getenv("EFFECTGO_TEST_ENGINE"); e != "" {
		if err := errset.Analyzer.Flags.Set("engine", e); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}

// Only the packages named here are analysis roots, where diagnostics and
// facts are checked. Their dependencies (domain, httpres) are analyzed for
// facts only.
func testdata(t *testing.T) string {
	t.Helper()
	return analysistest.TestData()
}

// setStubs sets the shared -stubs value (internal/stubs.Flag) for one test.
func setStubs(t *testing.T, value string) {
	t.Helper()
	old := stubs.Flag
	stubs.Flag = value
	t.Cleanup(func() { stubs.Flag = old })
}

// setFlag sets an analyzer flag for one test and restores its old value.
func setFlag(t *testing.T, name, value string) {
	t.Helper()
	old := errset.Analyzer.Flags.Lookup(name).Value.String()
	if err := errset.Analyzer.Flags.Set(name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = errset.Analyzer.Flags.Set(name, old) })
}

func TestSentinelsErrorTypesFmtJoinFlow(t *testing.T) {
	analysistest.Run(t, testdata(t), errset.Analyzer, "basic")
}

func TestErrmapCrossPackageAndHandlers(t *testing.T) {
	analysistest.Run(t, testdata(t), errset.Analyzer, "api", "badmap")
}

func TestContracts(t *testing.T) {
	analysistest.Run(t, testdata(t), errset.Analyzer, "contracts")
}

func TestOpaqueHandlerReported(t *testing.T) {
	analysistest.Run(t, testdata(t), errset.Analyzer, "opaque")
}

func TestIgnoreOpaque(t *testing.T) {
	setFlag(t, "ignoreopaque", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "opaqueignored")
}

func TestReportFlag(t *testing.T) {
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "report")
}

// TestASTFlowInsensitiveLimit pins the AST engine's flow-insensitive false
// positive (testdata/src/limitsast). The SSA engine, the default, does not have
// it (see ssaflow.guardReset).
func TestASTFlowInsensitiveLimit(t *testing.T) {
	setFlag(t, "engine", "ast")
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "limitsast")
}

// TestKnownLimitations pins behaviour that the analyzer does not handle
// precisely: interface calls, fields and func values.
func TestKnownLimitations(t *testing.T) {
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "limits")
}

// TestInterfaceContracts: a call to a contracted interface method yields the
// declared set, and a method-level directive overrides the type-level one.
func TestInterfaceContracts(t *testing.T) {
	analysistest.Run(t, testdata(t), errset.Analyzer, "ifaceuse")
}

// TestConformance: implementations are checked against the interface contracts.
func TestConformance(t *testing.T) {
	analysistest.Run(t, testdata(t), errset.Analyzer, "conform")
}

// TestIfaceCoverage: iface: variants are covered by an errmap whose errors.As
// target is an interface with the same or a subset of the methods.
func TestIfaceCoverage(t *testing.T) {
	analysistest.Run(t, testdata(t), errset.Analyzer, "ifaceweb")
}

// TestPolymorphism: param:i is substituted at call sites by literals, named
// functions and composed caller parameters.
func TestPolymorphism(t *testing.T) {
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "poly")
}

// TestStubs: stub rules override inference, contracts and std inference.
func TestStubs(t *testing.T) {
	setFlag(t, "report", "true")
	setStubs(t, filepath.Join(testdata(t), "stubs", "test.stubs"))
	analysistest.Run(t, testdata(t), errset.Analyzer, "stubuse")
}

// TestStatusInference covers constant StatusCode inference (ConstErr), the stub
// fallback (StubErr), HTTPErr{Status: 409} literals, passthrough, uncovered
// errors (500) and unknown statuses (default). See errset/status.go.
func TestStatusInference(t *testing.T) {
	setStubs(t, filepath.Join(testdata(t), "stubs", "status.stubs"))
	setFlag(t, "statuses", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "status")
}

// TestFuncValueStubs covers error stubs for a package-level func variable and a
// named func type, and a func literal called in place.
func TestFuncValueStubs(t *testing.T) {
	setStubs(t, filepath.Join(testdata(t), "stubs", "funcvals.stubs"))
	analysistest.Run(t, testdata(t), errset.Analyzer, "funcvals")
}

// TestFreshErrors: errors.New and a constant fmt.Errorf without %w are fresh
// ("new@pkg.F", one variant per creating function); a package-level error var
// stays a sentinel; "new" permits fresh variants, "?" does not; a fresh error
// is an unclassified error in a handler check.
func TestFreshErrors(t *testing.T) {
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "fresh")
}

// TestWitnessFlag: -witness prints one witness line per variant, with the
// chain below the reporting function ("(direct)" when there is none).
func TestWitnessFlag(t *testing.T) {
	setFlag(t, "witness", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "witness")
}

// TestInteractImmediateLiteral: an immediately-invoked literal's returns are its
// error set, and a fresh error it creates is keyed by the declared function that
// runs it (new@interact.Immediate), not by the literal.
func TestInteractImmediateLiteral(t *testing.T) {
	setFlag(t, "report", "true")
	setFlag(t, "witness", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "interact")
}

// TestRootSuppressed covers the root rule (testdata/src/root and rootdep): a
// contract violation whose only source is a violating callee is not reported;
// the other cases (a direct source, no contract, an allowing contract, a
// second callee that does not violate) are reported.
func TestRootSuppressed(t *testing.T) {
	analysistest.Run(t, testdata(t), errset.Analyzer, "root")
}

// TestRootSuppressedAll: -all reports the callers of a violating function too.
func TestRootSuppressedAll(t *testing.T) {
	setFlag(t, "all", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "rootall")
}

// TestShippedCause checks the rule for context.Cause in stubs/stdlib.stubs
// (testdata/src/cause): the sentinels, and an opaque cause.
func TestShippedCause(t *testing.T) {
	setFlag(t, "report", "true")
	setStubs(t, filepath.Join("..", "stubs", "stdlib.stubs"))
	analysistest.Run(t, testdata(t), errset.Analyzer, "cause")
}
