// Package goroot decides whether a package is part of the Go standard library
// from where its source files are, and never from the environment.
//
// A package is standard library iff its source directory is <root>/src/<import
// path> (or <root>/src/vendor/<import path> for the vendored golang.org/x
// packages) and <root> looks like a Go installation. The root is taken from the
// file path itself, so the answer does not depend on $GOROOT, on the toolchain
// that built the analyzer (runtime.GOROOT, go/build.Default.GOROOT), on the
// toolchain that go vet or go list selected (GOTOOLCHAIN), or on whether the
// environment is stale. Other source trees, for example the testdata layout
// <module>/testdata/src/<path> used by analysistest, have no such root and are
// user code.
package goroot

import (
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// rootCache maps a candidate root to whether it looks like a Go installation.
var rootCache sync.Map

// IsStdDir reports whether dir is the source directory of the standard library
// package importPath, that is <root>/src/<importPath> of a Go installation.
func IsStdDir(dir, importPath string) bool {
	if importPath == "" {
		return false
	}
	dir = filepath.Clean(dir)
	ip := filepath.FromSlash(importPath)
	for _, rel := range []string{filepath.Join("src", ip), filepath.Join("src", "vendor", ip)} {
		suffix := string(filepath.Separator) + rel
		if !strings.HasSuffix(dir, suffix) {
			continue
		}
		if isGoRoot(strings.TrimSuffix(dir, suffix)) {
			return true
		}
	}
	return false
}

// PkgIsStd reports whether pkg is a standard library package, judging by the
// source files that declare its objects. The fset must be the one in which pkg
// was type-checked or imported, so that object positions name real files.
// unsafe has no source file and is always standard library.
func PkgIsStd(fset *token.FileSet, pkg *types.Package) bool {
	if pkg == nil {
		return false
	}
	if pkg.Path() == "unsafe" {
		return true
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		pos := scope.Lookup(name).Pos()
		if !pos.IsValid() {
			continue
		}
		file := fset.Position(pos).Filename
		if file == "" {
			continue
		}
		if IsStdDir(filepath.Dir(file), pkg.Path()) {
			return true
		}
	}
	return false
}

// FilesAreStd reports whether any of the files named by filenames lies in the
// source directory of the standard library package importPath.
func FilesAreStd(importPath string, filenames []string) bool {
	for _, f := range filenames {
		if f != "" && IsStdDir(filepath.Dir(f), importPath) {
			return true
		}
	}
	return false
}

// isGoRoot reports whether root has the layout of a Go installation: a src/runtime
// package and either a VERSION file or a pkg/tool directory. The result is cached
// per root.
func isGoRoot(root string) bool {
	if root == "" {
		return false
	}
	if v, ok := rootCache.Load(root); ok {
		return v.(bool)
	}
	ok := isDir(filepath.Join(root, "src", "runtime")) &&
		(isFile(filepath.Join(root, "VERSION")) || isDir(filepath.Join(root, "pkg", "tool")))
	rootCache.Store(root, ok)
	return ok
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
