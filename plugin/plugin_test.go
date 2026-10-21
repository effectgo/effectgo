package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golangci/plugin-module-register/register"

	"github.com/effectgo/effectgo/errset"
	"github.com/effectgo/effectgo/internal/stubs"
	"github.com/effectgo/effectgo/mustuse"
)

// withStubs isolates the process-wide stub configuration of one test.
func withStubs(t *testing.T) {
	t.Helper()
	t.Setenv(stubs.Env, "")
	old := stubs.Flag
	t.Cleanup(func() { stubs.Flag = old })
	stubs.Flag = ""
}

func TestRegisteredUnderItsName(t *testing.T) {
	if _, err := register.GetPlugin(Name); err != nil {
		t.Fatalf("plugin %q is not registered: %v", Name, err)
	}
}

func TestAnalyzersAndLoadMode(t *testing.T) {
	p, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	as, err := p.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range as {
		names = append(names, a.Name)
	}
	if got := strings.Join(names, ","); got != "effects,errset,mustuse,exhaustive,schematag,directive" {
		t.Errorf("analyzers = %s, want effects,errset,mustuse,exhaustive,schematag,directive (report-only and HTTP analyzers are not exposed)", got)
	}
	if p.GetLoadMode() != register.LoadModeTypesInfo {
		t.Errorf("load mode = %q, want %q", p.GetLoadMode(), register.LoadModeTypesInfo)
	}
}

func TestSettingsDecodeStrictly(t *testing.T) {
	withStubs(t)
	if _, err := New(map[string]any{"stubz": []any{"x"}}); err == nil {
		t.Error("an unknown setting must be an error")
	}
}

func TestSettingsMustuseAllow(t *testing.T) {
	withStubs(t)
	old := mustuse.Analyzer.Flags.Lookup("allow").Value.String()
	t.Cleanup(func() { _ = mustuse.Analyzer.Flags.Set("allow", old) })
	if _, err := New(map[string]any{"mustuseAllow": []any{"(*os.File).Close", " ", "(*a.T).M"}}); err != nil {
		t.Fatal(err)
	}
	if got := mustuse.Analyzer.Flags.Lookup("allow").Value.String(); got != "(*os.File).Close,(*a.T).M" {
		t.Errorf("mustuse -allow = %q, want the two non-blank names", got)
	}
}

func TestSettingsStubsAndIgnoreOpaque(t *testing.T) {
	withStubs(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	abs := filepath.Join(dir, "abs.stubs")
	if _, err := New(map[string]any{
		"stubs":        []any{"rel/a.stubs", abs},
		"ignoreOpaque": true,
	}); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{filepath.Join(dir, "rel", "a.stubs"), abs}, ",")
	if stubs.Flag != want {
		t.Errorf("stubs.Flag = %q, want %q (relative paths resolve against the module root)", stubs.Flag, want)
	}
	if got := errset.Analyzer.Flags.Lookup("ignoreopaque").Value.String(); got != "true" {
		t.Errorf("ignoreopaque = %s, want true", got)
	}
	// Restore the default for the other tests of this package.
	if _, err := New(map[string]any{"ignoreOpaque": false}); err != nil {
		t.Fatal(err)
	}
}

func TestNoStubsSettingKeepsZeroConfiguration(t *testing.T) {
	withStubs(t)
	if _, err := New(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if stubs.Flag != "" {
		t.Errorf("stubs.Flag = %q, want empty (zero configuration)", stubs.Flag)
	}
}
