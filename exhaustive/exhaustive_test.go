package exhaustive_test

import (
	"testing"

	"github.com/faintghost/effectgo/exhaustive"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestExhaustive(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), exhaustive.Analyzer, "shape", "client", "enum", "enumuse", "args")
}
