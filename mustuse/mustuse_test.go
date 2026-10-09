package mustuse_test

import (
	"testing"

	"github.com/faintghost/effectgo/mustuse"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestMustUse(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "a", "b", "c", "d")
}

// TestMustUseAllowFlag allows the method (*os.File).Close by its full name. The
// call, the method value and the func variable that holds it are then silent;
// an unresolvable func variable is still reported.
func TestMustUseAllowFlag(t *testing.T) {
	old := mustuse.Analyzer.Flags.Lookup("allow").Value.String()
	if err := mustuse.Analyzer.Flags.Set("allow", "(*os.File).Close"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mustuse.Analyzer.Flags.Set("allow", old) })
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "e")
}
