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

// The predicate rule shared with errors.As: an early constant false return exports
// the errors.As test, which is a known false negative (testdata/src/predlimit,
// CallerAs), and the errors.AsType form exports nothing there, so it is reported.
func TestPredicateEarlyFalseLimit(t *testing.T) {
	setFlag(t, "engine", "ssa")
	setFlag(t, "report", "true")
	analysistest.Run(t, testdata(t), errset.Analyzer, "predlimit")
}

// The errmap reads errors.AsType and the handler check uses its mapped variants
// (testdata/src/astypemap and astypehandler). The errmap is read by the shared AST
// reader, and the handler set by each engine, so both engines run.
func TestErrmapAsType(t *testing.T) {
	for _, engine := range []string{"ssa", "ast"} {
		t.Run(engine, func(t *testing.T) {
			setFlag(t, "engine", engine)
			analysistest.Run(t, testdata(t), errset.Analyzer, "astypehandler")
		})
	}
}
