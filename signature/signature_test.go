package signature_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/effectgo/effectgo/signature"
)

// TestSignature checks the Effect<A, E, R> line of every declared function.
// The effects and errset analyzers run as required analyzers; their own
// diagnostics (here, the violated //effectgo:pure on Clock) are not printed,
// since only the signature analyzer is a root.
func TestSignature(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), signature.Analyzer, "sigs")
}
