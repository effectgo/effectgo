package errset_test

import (
	"path/filepath"
	"testing"

	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
	"golang.org/x/tools/go/analysis/analysistest"
)

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

// TestKnownLimitations pins behaviour that the analyzer does not handle
// precisely: flow-insensitivity, interface calls, fields and func values.
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
