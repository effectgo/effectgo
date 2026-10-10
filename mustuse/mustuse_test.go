package mustuse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faintghost/effectgo/internal/stubs"
	"github.com/faintghost/effectgo/mustuse"
	"golang.org/x/tools/go/analysis/analysistest"
)

// TestMain clears EFFECTGO_STUBS and selects an explicit empty stub file: each
// test states its stub files itself. An empty -stubs would be the zero
// configuration, which loads the shipped stdlib.stubs and its writer rules, so
// the tests without stubs would not see the unstubbed behaviour.
func TestMain(m *testing.M) {
	os.Unsetenv(stubs.Env)
	none, err := filepath.Abs("testdata/stubs/none.stubs")
	if err != nil {
		panic(err)
	}
	stubs.Flag = none
	os.Exit(m.Run())
}

func TestMustUse(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "a", "b", "c", "d", "fn")
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
// file, which states "errors ... none" for them. TestWriterStubsZeroConfig runs
// the same calls with no -stubs and no $EFFECTGO_STUBS: the zero configuration
// loads the shipped files, so they are silent too. TestWriterStubsAbsent runs
// them with the empty stub file: they are reported.
func TestWriterStubs(t *testing.T) {
	setStubs(t, stdlibStubs(t))
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "w")
}

func TestWriterStubsZeroConfig(t *testing.T) {
	setStubs(t, "")
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "w")
}

func TestWriterStubsAbsent(t *testing.T) {
	setStubs(t, noStubs(t))
	analysistest.Run(t, analysistest.TestData(), mustuse.Analyzer, "wn")
}

// noStubs is the absolute path of the empty stub file that TestMain pins.
func noStubs(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("testdata/stubs/none.stubs")
	if err != nil {
		t.Fatal(err)
	}
	return p
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
