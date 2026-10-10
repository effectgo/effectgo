package errset_test

import (
	"testing"

	"github.com/faintghost/effectgo/errset"
	"golang.org/x/tools/go/analysis/analysistest"
)

// The SSA engine's flow-sensitive expectations (testdata/src/ssaflow).
func TestSSAFlowSensitive(t *testing.T) {
	setFlag(t, "engine", "ssa")
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "ssaflow")
}

// Named results captured or deferred (testdata/src/deferflow): a deferred
// closure that reads the result does not widen it; one that writes it adds
// what it writes.
func TestSSADeferredNamedResults(t *testing.T) {
	setFlag(t, "engine", "ssa")
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "deferflow")
}
