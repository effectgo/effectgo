package goroot

import (
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"
)

// fakeGoRoot lays out a minimal Go installation: src/runtime, VERSION and the
// package src/fakestd. It returns the root and the file of fakestd.
func fakeGoRoot(t *testing.T) (root, fakestdFile string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "go1.26.9")
	write(t, filepath.Join(root, "VERSION"), "go1.26.9\n")
	write(t, filepath.Join(root, "src", "runtime", "runtime.go"), "package runtime\n")
	fakestdFile = filepath.Join(root, "src", "fakestd", "x.go")
	write(t, fakestdFile, "package fakestd\n\nconst C = 1\n")
	return root, fakestdFile
}

// testdataLayout lays out an analysistest-style GOPATH tree: <mod>/testdata/src/a.
func testdataLayout(t *testing.T) string {
	t.Helper()
	mod := filepath.Join(t.TempDir(), "analyzer")
	write(t, filepath.Join(mod, "testdata", "src", "a", "a.go"), "package a\n")
	return filepath.Join(mod, "testdata", "src", "a", "a.go")
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// envSettings are the GOROOT values under which the stdlib check must give the
// same answer: unset, stale (another existing or a nonexistent directory), and
// correct.
func envSettings(t *testing.T, root string) map[string]string {
	t.Helper()
	return map[string]string{
		"unset":       "",
		"stale-other": t.TempDir(),
		"stale-gone":  filepath.Join(t.TempDir(), "gone"),
		"correct":     root,
	}
}

func TestStdlibRecognizedWhateverGOROOTIs(t *testing.T) {
	root, file := fakeGoRoot(t)
	for name, val := range envSettings(t, root) {
		t.Run(name, func(t *testing.T) {
			if val == "" {
				t.Setenv("GOROOT", "")
				os.Unsetenv("GOROOT")
			} else {
				t.Setenv("GOROOT", val)
			}
			if !IsStdDir(filepath.Dir(file), "fakestd") {
				t.Errorf("IsStdDir(fakestd) = false with GOROOT=%q", val)
			}
			if !FilesAreStd("fakestd", []string{file}) {
				t.Errorf("FilesAreStd(fakestd) = false with GOROOT=%q", val)
			}
			if IsStdDir(filepath.Join(root, "src", "fakestd"), "other") {
				t.Errorf("a directory is std for a different import path")
			}

			fset := token.NewFileSet()
			pkg := types.NewPackage("fakestd", "fakestd")
			tf := fset.AddFile(file, -1, 64)
			obj := types.NewConst(token.Pos(tf.Base()), pkg, "C", types.Typ[types.Int], nil)
			pkg.Scope().Insert(obj)
			if !PkgIsStd(fset, pkg) {
				t.Errorf("PkgIsStd(fakestd) = false with GOROOT=%q", val)
			}
		})
	}
}

func TestStdlibCheckIsCachedPerRoot(t *testing.T) {
	root, file := fakeGoRoot(t)
	for range 3 {
		if !IsStdDir(filepath.Dir(file), "fakestd") {
			t.Fatal("IsStdDir(fakestd) = false")
		}
	}
	if !isGoRoot(root) {
		t.Fatal("root not recognized")
	}
	if v, ok := rootCache.Load(root); !ok || !v.(bool) {
		t.Fatal("root result not cached")
	}
}

func TestNonGoRootLayoutIsNotStd(t *testing.T) {
	// A src/fakestd directory without src/runtime or VERSION is not an installation.
	dir := filepath.Join(t.TempDir(), "proj", "src", "fakestd")
	write(t, filepath.Join(dir, "x.go"), "package fakestd\n")
	if IsStdDir(dir, "fakestd") {
		t.Error("a src tree without a Go installation layout is std")
	}
}

func TestTestdataPackagesAreNotStd(t *testing.T) {
	file := testdataLayout(t)
	for name, val := range envSettings(t, filepath.Dir(file)) {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GOROOT", val)
			if IsStdDir(filepath.Dir(file), "a") {
				t.Errorf("testdata package a is std with GOROOT=%q", val)
			}
			if FilesAreStd("a", []string{file}) {
				t.Errorf("FilesAreStd(testdata a) = true with GOROOT=%q", val)
			}
			fset := token.NewFileSet()
			pkg := types.NewPackage("a", "a")
			tf := fset.AddFile(file, -1, 16)
			pkg.Scope().Insert(types.NewConst(token.Pos(tf.Base()), pkg, "C", types.Typ[types.Int], nil))
			if PkgIsStd(fset, pkg) {
				t.Errorf("PkgIsStd(testdata a) = true")
			}
		})
	}
}

func TestRepoTestdataPackagesAreNotStd(t *testing.T) {
	// The real analysistest data of this repository, next to the package under test.
	file, err := filepath.Abs("../../effects/testdata/src/effs/effs.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Skip("repository testdata not present")
	}
	if IsStdDir(filepath.Dir(file), "effs") || FilesAreStd("effs", []string{file}) {
		t.Error("repository testdata package effs is std")
	}
}

func TestUnsafeIsStd(t *testing.T) {
	if !PkgIsStd(token.NewFileSet(), types.Unsafe) {
		t.Error("unsafe is not std")
	}
	if PkgIsStd(token.NewFileSet(), nil) {
		t.Error("nil package is std")
	}
}
