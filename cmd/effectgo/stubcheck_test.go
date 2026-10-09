package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// module writes a module with an effectgo.stubs file (text) and returns its root.
func module(t *testing.T, stubsText string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "effectgo.stubs"), []byte(stubsText), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EFFECTGO_STUBS", "")
	return dir
}

// A project rule for a target the shipped set also has replaces it: no error.
func TestCheckStubsProjectOverridesShipped(t *testing.T) {
	dir := module(t, "effects (context.Context).Done net\n")
	if err := checkStubs([]string{"effectgo", filepath.Join(dir, "...")}); err != nil {
		t.Fatalf("override of a shipped rule: %v", err)
	}
}

// Two rules of one project file: one error, naming the first definition.
func TestCheckStubsDuplicateInOneLayer(t *testing.T) {
	dir := module(t, "effects time.Now time\neffects time.Now pure\n")
	err := checkStubs([]string{"effectgo", dir + "/..."})
	if err == nil {
		t.Fatal("duplicate in one layer: no error")
	}
	want := "effectgo.stubs:2: duplicate effects rule for time.Now, first defined at " + filepath.Join(dir, "effectgo.stubs") + ":1"
	if !strings.HasSuffix(err.Error(), want) {
		t.Errorf("error = %q, want suffix %q", err.Error(), want)
	}
}

// An explicit -stubs file is checked, and go vet invocations are not.
func TestCheckStubsExplicitAndVet(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.stubs")
	if err := os.WriteFile(bad, []byte("effects time.Now nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := checkStubs([]string{"effectgo", "-stubs=" + bad, "./..."})
	if err == nil || !strings.Contains(err.Error(), "bad.stubs:1: unknown effect name") {
		t.Errorf("explicit bad file: error %v", err)
	}
	if err := checkStubs([]string{"effectgo", "-stubs", bad, "./..."}); err == nil {
		t.Error("explicit bad file (separate value): no error")
	}
	if err := checkStubs([]string{"effectgo", "-stubs=" + bad, "/tmp/unit.cfg"}); err != nil {
		t.Errorf("vet unit should not be checked: %v", err)
	}
	if err := checkStubs([]string{"effectgo", "-flags"}); err != nil {
		t.Errorf("-flags should not be checked: %v", err)
	}
}
