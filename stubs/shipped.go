// Package stubs embeds the stub files that ship with effectgo, so that the
// analyzers, the go vet tool and the golangci-lint plugin work without a
// checkout of this repository. The files are the *.stubs next to this file.
// internal/stubs reads them through Files; the files themselves are the
// documentation of the rules and are read as text, as the stub format is.
package stubs

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
)

// The stub coverage page is generated from these files; go generate runs
// internal/cmd/stubdoc, and its test fails when the page is out of date.
//
//go:generate go run ../internal/cmd/stubdoc -o ../docs/reference/stub-coverage.md

//go:embed *.stubs
var files embed.FS

// File is one shipped stub file: its base name and its text.
type File struct {
	Name string
	Data []byte
}

// Files returns the shipped stub files, sorted by name.
func Files() ([]File, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("shipped stubs: %w", err)
	}
	var out []File
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".stubs" {
			continue
		}
		data, err := files.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("shipped stubs: %w", err)
		}
		out = append(out, File{Name: e.Name(), Data: data})
	}
	return out, nil
}
