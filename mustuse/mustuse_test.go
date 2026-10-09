package mustuse_test

import (
	"testing"

	"github.com/faintghost/effectgo/mustuse"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestMustUse(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "a", "b", "c")
}
