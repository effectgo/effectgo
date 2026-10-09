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
