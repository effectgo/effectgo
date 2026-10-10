package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faintghost/effectgo/internal/stubs"
)

// TestIdentityCoversProjectStubs: the vet cache identity changes when the
// project file of the module changes, and when a configured stub file changes,
// and it does not change when nothing does.
func TestIdentityCoversProjectStubs(t *testing.T) {
	t.Setenv(stubs.Env, "")
	old := stubs.Flag
	stubs.Flag = ""
	t.Cleanup(func() { stubs.Flag = old })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(dir, stubs.ProjectFile)
	if err := os.WriteFile(project, []byte("effects pkg.F pure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := identityHash(dir)
	if again := identityHash(dir); again != before {
		t.Fatalf("identity is not stable: %s vs %s", before, again)
	}
	if err := os.WriteFile(project, []byte("effects pkg.F time\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if after := identityHash(dir); after == before {
		t.Fatal("editing the project stub file did not change the identity")
	}

	// Without the project file the identity differs from the one with it.
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}
	if without := identityHash(dir); without == before {
		t.Fatal("removing the project stub file did not change the identity")
	}

	// An explicit stub file replaces the zero configuration and is hashed.
	explicit := filepath.Join(dir, "x.stubs")
	if err := os.WriteFile(explicit, []byte("effects pkg.G pure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubs.Flag = explicit
	withExplicit := identityHash(dir)
	if err := os.WriteFile(explicit, []byte("effects pkg.G time\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if identityHash(dir) == withExplicit {
		t.Fatal("editing an explicit stub file did not change the identity")
	}
}
