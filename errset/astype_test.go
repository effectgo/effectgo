package errset_test

import (
	"testing"

	"github.com/faintghost/effectgo/errset"
	"golang.org/x/tools/go/analysis/analysistest"
)

// errors.AsType narrows like errors.As (testdata/src/astype, with the predicate
// package astypeprobe): each form of the ok result on both edges, value and
// interface targets, wrappers, and the predicates of this package and another.
// The SSA engine, the default.
func TestSSAAsTypeNarrowing(t *testing.T) {
	setFlag(t, "engine", "ssa")
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "astype")
}

// The AST engine does not narrow on errors.AsType, as it does not narrow on
// errors.As (testdata/src/limitsasttype).
func TestASTAsTypeLimit(t *testing.T) {
	setFlag(t, "engine", "ast")
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "limitsasttype")
}

// The errmap reads errors.AsType and the handler check uses its mapped variants
// (testdata/src/astypemap and astypehandler). The errmap reader is the AST one,
// so this runs under both engines.
func TestErrmapAsType(t *testing.T) {
	for _, engine := range []string{"ssa", "ast"} {
		t.Run(engine, func(t *testing.T) {
			setFlag(t, "engine", engine)
			analysistest.Run(t, testdata(t), errset.Analyzer, "astypehandler")
		})
	}
}
