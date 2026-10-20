package stubs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newModule makes a module directory with go.mod and returns its path.
func newModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// unconfigured clears -stubs and $EFFECTGO_STUBS for one test.
func unconfigured(t *testing.T) {
	t.Helper()
	t.Setenv(Env, "")
	old := Flag
	Flag = ""
	t.Cleanup(func() { Flag = old })
}

func names(srcs []Source) []string {
	var out []string
	for _, s := range srcs {
		out = append(out, s.Name)
	}
	return out
}

func TestZeroConfigShippedAndProjectFile(t *testing.T) {
	unconfigured(t)
	dir := newModule(t)
	sub := filepath.Join(dir, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(dir, ProjectFile)
	if err := os.WriteFile(project, []byte("effects m.F pure\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := names(mustSources(t, filepath.Join(sub, "x.go")))
	if len(got) < 2 || !strings.HasPrefix(got[0], ShippedPrefix) {
		t.Fatalf("shipped files come first, got %v", got)
	}
	if last := got[len(got)-1]; last != project {
		t.Fatalf("project file is the last source: got %v", got)
	}
	for _, n := range got[:len(got)-1] {
		if !strings.HasPrefix(n, ShippedPrefix) {
			t.Errorf("source %q is not a shipped file", n)
		}
	}
	rules, err := RulesFor(sub)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rules {
		if r.File == project {
			found = true
		}
	}
	if !found {
		t.Errorf("rules of the project file are missing")
	}
}

func TestNoProjectFileOutsideModule(t *testing.T) {
	unconfigured(t)
	dir := t.TempDir()
	if ModuleRoot(dir) != "" {
		t.Skip("the temp directory is inside a module on this machine")
	}
	for _, s := range mustSources(t, dir) {
		if strings.HasSuffix(s.Name, ProjectFile) {
			t.Fatalf("project file found outside any module: %s", s.Name)
		}
	}
}

func TestExplicitStubsDisableDiscovery(t *testing.T) {
	unconfigured(t)
	dir := newModule(t)
	project := filepath.Join(dir, ProjectFile)
	if err := os.WriteFile(project, []byte("effects m.F pure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(dir, "mine.stubs")
	if err := os.WriteFile(explicit, []byte("effects m.G pure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Flag = explicit
	got := names(mustSources(t, dir))
	if len(got) != 1 || got[0] != explicit {
		t.Fatalf("explicit -stubs must replace the zero configuration, got %v", got)
	}
}

func TestExplicitMissingFileIsAnError(t *testing.T) {
	unconfigured(t)
	Flag = filepath.Join(t.TempDir(), "missing.stubs")
	if _, err := Sources(t.TempDir()); err == nil {
		t.Fatal("a missing explicit stub file must be an error")
	}
}

func TestMalformedProjectFileNamesFile(t *testing.T) {
	unconfigured(t)
	dir := newModule(t)
	project := filepath.Join(dir, ProjectFile)
	if err := os.WriteFile(project, []byte("effects broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := RulesFor(dir)
	if err == nil || !strings.Contains(err.Error(), project) {
		t.Fatalf("want an error naming %s, got %v", project, err)
	}
}

func TestModuleRoot(t *testing.T) {
	dir := newModule(t)
	deep := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(deep, "f.go")
	if err := os.WriteFile(file, []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ModuleRoot(file); got != dir {
		t.Errorf("ModuleRoot(file) = %q, want %q", got, dir)
	}
	if got := ModuleRoot(deep); got != dir {
		t.Errorf("ModuleRoot(dir) = %q, want %q", got, dir)
	}
}

func TestShippedFilesParse(t *testing.T) {
	unconfigured(t)
	srcs := mustSources(t, t.TempDir())
	if len(srcs) != 6 {
		t.Fatalf("want 6 shipped stub files, got %d: %v", len(srcs), names(srcs))
	}
	for _, s := range srcs {
		if _, err := Parse(s.Name, s.Data); err != nil {
			t.Errorf("%s: %v", s.Name, err)
		}
	}
}

func mustSources(t *testing.T, p string) []Source {
	t.Helper()
	srcs, err := Sources(p)
	if err != nil {
		t.Fatalf("Sources(%s): %v", p, err)
	}
	return srcs
}
