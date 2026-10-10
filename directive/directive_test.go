package directive_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/faintghost/effectgo/directive"
)

// The placements that some analyzer reads stay silent here; the other analyzers'
// own diagnostics for their placements are not expected by this analyzer. The
// combined binary is checked end to end in cmd/effectgo (inert_test.go).

func TestPlacement(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), directive.Analyzer, "place", "intest", "pkgdoc")
}

func TestSpacing(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), directive.Analyzer, "spacing")
}
