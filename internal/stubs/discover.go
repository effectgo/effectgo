package stubs

// Which stub files an analysis reads, decided per analyzed package:
//
//   - -stubs, or $EFFECTGO_STUBS when -stubs is empty: exactly those files.
//     Nothing else is read (this is the explicit configuration, as before).
//   - otherwise (zero configuration): the stub files shipped inside the binary
//     (the embedded *.stubs of the stubs package), then the project file
//     <module root>/effectgo.stubs, when the module of the analyzed code has one.
//
// The module root is the nearest directory at or above the analyzed file that
// holds a go.mod. Files outside any module have no project file.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"

	shipped "github.com/faintghost/effectgo/stubs"
)

// ProjectFile is the name of the stub file that a module keeps at its root.
const ProjectFile = "effectgo.stubs"

// ShippedPrefix marks the names of the shipped files in error messages and in
// the vet identity, so they cannot be mistaken for files on disk.
const ShippedPrefix = "effectgo:"

// IsShipped reports whether a rule's file name (Rule.File) is one of the stub
// files embedded in the binary.
func IsShipped(name string) bool {
	return strings.HasPrefix(name, ShippedPrefix)
}

// Where renders a rule's position for a message: stubs/<file>:<line> for a
// shipped file (the file of the repository, whose name is the base name), else
// the path and line as given.
func Where(name string, line int) string {
	if IsShipped(name) {
		return fmt.Sprintf("stubs/%s:%d", strings.TrimPrefix(name, ShippedPrefix), line)
	}
	return fmt.Sprintf("%s:%d", name, line)
}

// Source is one stub file: its name (a path, or ShippedPrefix plus the base
// name of a shipped file) and its text.
type Source struct {
	Name string
	Data []byte
}

// Configured reports whether the stub files are given explicitly (-stubs or
// $EFFECTGO_STUBS). When it is false, the zero-configuration set is used.
func Configured() bool {
	return len(Paths()) > 0
}

// Sources returns the stub files for the analysis of the code at p, which is a
// file or a directory. See the comment at the top of this file.
func Sources(p string) ([]Source, error) {
	if Configured() {
		var out []Source
		for _, name := range Paths() {
			data, err := os.ReadFile(name)
			if err != nil {
				return nil, fmt.Errorf("stub file: %w", err)
			}
			out = append(out, Source{Name: name, Data: data})
		}
		return out, nil
	}
	files, err := shipped.Files()
	if err != nil {
		return nil, err
	}
	var out []Source
	for _, f := range files {
		out = append(out, Source{Name: ShippedPrefix + f.Name, Data: f.Data})
	}
	root := ModuleRoot(p)
	if root == "" {
		return out, nil
	}
	name := filepath.Join(root, ProjectFile)
	data, err := os.ReadFile(name)
	switch {
	case err == nil:
		out = append(out, Source{Name: name, Data: data})
	case errors.Is(err, fs.ErrNotExist):
	default:
		return nil, fmt.Errorf("stub file: %w", err)
	}
	return out, nil
}

// RulesFor parses the stub files of Sources(p). Malformed rules are errors
// that name the file and the line.
func RulesFor(p string) ([]Rule, error) {
	srcs, err := Sources(p)
	if err != nil {
		return nil, err
	}
	var all []Rule
	for _, s := range srcs {
		rules, err := Parse(s.Name, s.Data)
		if err != nil {
			return nil, err
		}
		all = append(all, rules...)
	}
	return all, nil
}

// RulesForPass is RulesFor for the package of an analysis pass. A pass without
// files falls back to the working directory.
func RulesForPass(pass *analysis.Pass) ([]Rule, error) {
	return RulesFor(PassPath(pass))
}

// PassPath is the path of one file of the pass's package, or "." when the pass
// has none.
func PassPath(pass *analysis.Pass) string {
	if len(pass.Files) > 0 {
		if name := pass.Fset.Position(pass.Files[0].Pos()).Filename; name != "" {
			return name
		}
	}
	return "."
}

// ModuleRoot returns the directory of the nearest go.mod at or above p (a file
// or a directory), or "" when there is none.
func ModuleRoot(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	dir := abs
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
