package tsgen

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/effectgo/effectgo/internal/stubs"
)

const optionsPrefix = "// contract-options: "

type contractOptions struct {
	Version  int      `json:"version"`
	Patterns []string `json:"patterns"`
}

// Generate writes the client with portable package-selection options for checks.
func (c Collected) Generate(api string) (string, error) {
	text, err := Generate(c.Results, api, c.Module, c.Packages)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(contractOptions{Version: 1, Patterns: c.patterns})
	if err != nil {
		return "", err
	}
	first, rest, _ := strings.Cut(text, "\n")
	return first + "\n" + optionsPrefix + string(body) + "\n" + rest, nil
}

// portablePatterns makes local patterns relative to the generating module so
// checks work from its root, even when generation ran in a package directory.
func portablePatterns(dir string, patterns []string) ([]string, error) {
	base, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	root := stubs.ModuleRoot(base)
	if root == "" {
		return nil, fmt.Errorf("the packages are not in a module")
	}
	portable := make([]string, len(patterns))
	for i, pattern := range patterns {
		if filepath.IsAbs(pattern) || pattern == "." || pattern == ".." || strings.HasPrefix(pattern, "./") || strings.HasPrefix(pattern, "../") {
			path := pattern
			if !filepath.IsAbs(path) {
				path = filepath.Join(base, path)
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return nil, err
			}
			pattern = "./" + filepath.ToSlash(relative)
		}
		portable[i] = pattern
	}
	return portable, nil
}

// CheckFingerprint analyzes the original package selection and compares its
// contract with the generated hash without generating or rewriting TypeScript.
func CheckFingerprint(root string, generated []byte) (string, error) {
	var options contractOptions
	contract := ""
	for line := range strings.SplitSeq(string(generated), "\n") {
		if value, ok := strings.CutPrefix(line, "// contract-fingerprint: sha256:"); ok {
			contract = value
		}
		if value, ok := strings.CutPrefix(line, optionsPrefix); ok {
			if err := json.Unmarshal([]byte(value), &options); err != nil {
				return "", fmt.Errorf("invalid contract fingerprint options: %w", err)
			}
		}
	}
	hash, err := hex.DecodeString(contract)
	if err != nil || len(hash) != 32 || options.Version != 1 || len(options.Patterns) == 0 {
		return "", fmt.Errorf("missing or unsupported contract fingerprint; regenerate with effectgo ts")
	}
	collected, err := Collect(root, options.Patterns...)
	if err != nil {
		return "", fmt.Errorf("stale contract fingerprint; cannot analyze current contract: %w", err)
	}
	current := Fingerprint(collected.Results)
	if current != contract {
		return current, fmt.Errorf("stale contract fingerprint; regenerate with effectgo ts")
	}
	return current, nil
}
