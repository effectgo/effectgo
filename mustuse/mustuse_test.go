package mustuse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faintghost/effectgo/internal/stubs"
	"github.com/faintghost/effectgo/mustuse"
	"golang.org/x/tools/go/analysis/analysistest"
)

// TestMain clears EFFECTGO_STUBS: each test states its stub files itself.
func TestMain(m *testing.M) {
	os.Unsetenv(stubs.Env)
	os.Exit(m.Run())
}

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

// TestWriterStubs runs the writers of package w with the shipped stdlib stub
// file, which states "errors ... none" for them. TestWriterStubsAbsent runs the
// same calls without stub files: they are reported.
func TestWriterStubs(t *testing.T) {
	setStubs(t, stdlibStubs(t))
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "w")
}

func TestWriterStubsAbsent(t *testing.T) {
	setStubs(t, "")
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "wn")
}

// stdlibStubs is the absolute path of the shipped stdlib.stubs.
func stdlibStubs(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../stubs/stdlib.stubs")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// setStubs sets the shared -stubs value for one test.
func setStubs(t *testing.T, value string) {
	t.Helper()
	old := stubs.Flag
	stubs.Flag = value
	t.Cleanup(func() { stubs.Flag = old })
}
