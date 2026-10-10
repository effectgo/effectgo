package effects_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/faintghost/effectgo/effects"
)

// TestViaItems covers via items of the shipped stub rules (testdata/src/via):
// a writer or reader argument is evaluated at the call site from its static
// type, and an interface argument is unknown or its contract.
func TestViaItems(t *testing.T) {
	setStubs(t, absPath(t, "../stubs/stdlib.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "via", "via/local")
}

// TestStoredValues covers stores items and via:stored items (testdata/src/stored).
func TestStoredValues(t *testing.T) {
	setStubs(t, absPath(t, "../stubs/stdlib.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "stored")
}

// TestFieldSources covers field source rules of the shipped stub set
// (testdata/src/fieldsrc): a method on, or a reader built from, the body field of
// an http.Request or http.Response is net. Another field, a reassigned local and
// an address-taken local stay unknown.
func TestFieldSources(t *testing.T) {
	setStubs(t, absPath(t, "../stubs/stdlib.stubs"))
	analysistest.Run(t, analysistest.TestData(), effects.Analyzer, "fieldsrc")
}
