package tsgen

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/build"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/effectgo/effectgo/internal/stubs"
	"github.com/effectgo/effectgo/internal/version"
)

const inputsPrefix = "// effectgo-inputs: "

// SourceInputs caches the source identity of an analyzed contract. A changed
// source identity requires analysis, not regeneration, when the contract agrees.
// Paths are relative to the generating module, including local replacements.
type SourceInputs struct {
	Version  int      `json:"version"`
	Contract string   `json:"contract"`
	Digest   string   `json:"digest"`
	Roots    []string `json:"roots"`
	Files    []string `json:"files"`
	Patterns []string `json:"patterns"`
}

func collectSourceInputs(dir string, pkgs []*packages.Package) (*SourceInputs, error) {
	root := stubs.ModuleRoot(dir)
	if root == "" {
		return nil, fmt.Errorf("the packages are not in a module")
	}
	inputs := &SourceInputs{Version: 1}
	roots := map[string]bool{root: true}
	files := map[string]bool{}
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if module := pkg.Module; module != nil {
			if module.Replace != nil {
				module = module.Replace
			}
			if module.Version == "" && module.Dir != "" {
				roots[module.Dir] = true
				for _, file := range pkg.EmbedFiles {
					files[file] = true
				}
			}
		}
	})
	for moduleRoot := range roots {
		inputs.Roots = append(inputs.Roots, relativeInput(root, moduleRoot))
		sources, err := stubs.Sources(moduleRoot)
		if err != nil {
			return nil, err
		}
		for _, source := range sources {
			if !stubs.IsShipped(source.Name) {
				files[source.Name] = true
			}
		}
	}
	config, err := sourceConfig(root)
	if err != nil {
		return nil, err
	}
	if workspace := config["GOWORK"]; workspace != "" && workspace != "off" {
		files[filepath.Join(root, filepath.FromSlash(workspace))] = true
		files[filepath.Join(root, filepath.FromSlash(workspace))+".sum"] = true
	}
	for file := range files {
		inputs.Files = append(inputs.Files, relativeInput(root, file))
	}
	slices.Sort(inputs.Roots)
	slices.Sort(inputs.Files)
	inputs.Digest, err = inputs.current(root)
	return inputs, err
}

func relativeInput(root, name string) string {
	if !filepath.IsAbs(name) {
		absolute, err := filepath.Abs(name)
		if err == nil {
			name = absolute
		}
	}
	name, err := filepath.Rel(root, name)
	if err != nil {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(name)
}

func sourceConfig(root string) (map[string]string, error) {
	// go env resolves defaults and GOENV without loading or analyzing packages.
	cmd := exec.Command("go", "env", "-json", "GOOS", "GOARCH", "GOFLAGS", "CGO_ENABLED", "GOVERSION", "GOEXPERIMENT", "GO111MODULE", "GOWORK", "GOAMD64", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM", "GOFIPS140")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fingerprint build configuration: %w", err)
	}
	var config map[string]string
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, err
	}
	if workspace := config["GOWORK"]; workspace != "" && workspace != "off" {
		config["GOWORK"] = relativeInput(root, workspace)
	} else {
		config["GOWORK"] = ""
	}
	var configured []string
	for _, file := range stubs.Paths() {
		configured = append(configured, relativeInput(root, file))
	}
	config["stubs"] = strings.Join(configured, ",")
	config["effectgo"] = version.Current
	config["target"] = Target
	sources, err := stubs.Sources(root)
	if err != nil {
		return nil, err
	}
	for _, source := range sources {
		if stubs.IsShipped(source.Name) {
			config[source.Name] = fmt.Sprintf("%x", sha256.Sum256(source.Data))
		}
	}
	flags, err := fingerprintFlags(config["GOFLAGS"])
	if err != nil {
		return nil, err
	}
	var semanticFlags []string
	for _, flag := range flags {
		name, file, ok := strings.Cut(flag, "=")
		switch name {
		case "-tags", "-race", "-msan", "-asan", "-compiler":
			semanticFlags = append(semanticFlags, flag)
		}
		if !ok || name != "-modfile" && name != "-overlay" {
			continue
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(root, file)
		}
		semanticFlags = append(semanticFlags, name+"="+relativeInput(root, file))
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if name == "-modfile" {
			config[name] = fmt.Sprintf("%x", sha256.Sum256(body))
			sum, err := os.ReadFile(strings.TrimSuffix(file, ".mod") + ".sum")
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			config["-modfile-sum"] = fmt.Sprintf("%x", sha256.Sum256(sum))
			continue
		}
		var overlay struct{ Replace map[string]string }
		if err := json.Unmarshal(body, &overlay); err != nil {
			return nil, err
		}
		for original, replacement := range overlay.Replace {
			if replacement == "" {
				config["overlay:"+relativeInput(root, original)] = "deleted"
				continue
			}
			if !filepath.IsAbs(replacement) {
				replacement = filepath.Join(root, replacement)
			}
			body, err := os.ReadFile(replacement)
			if err != nil {
				return nil, err
			}
			config["overlay:"+relativeInput(root, original)] = fmt.Sprintf("%x", sha256.Sum256(body))
		}
	}
	encodedFlags, err := json.Marshal(semanticFlags)
	if err != nil {
		return nil, err
	}
	config["GOFLAGS"] = string(encodedFlags)
	return config, nil
}

// GOFLAGS uses Go's quoted-field syntax: a field may start with a single or
// double quote; there are no escape sequences within quoted fields.
func fingerprintFlags(text string) ([]string, error) {
	var flags []string
	for text = strings.TrimSpace(text); text != ""; text = strings.TrimSpace(text) {
		if text[0] == '\'' || text[0] == '"' {
			quote := text[0]
			end := strings.IndexByte(text[1:], quote)
			if end < 0 {
				return nil, fmt.Errorf("unterminated quote in GOFLAGS")
			}
			flags = append(flags, text[1:end+1])
			text = text[end+2:]
			continue
		}
		end := strings.IndexAny(text, " \t\r\n")
		if end < 0 {
			flags = append(flags, text)
			break
		}
		flags = append(flags, text[:end])
		text = text[end:]
	}
	return flags, nil
}

func (inputs *SourceInputs) current(root string) (string, error) {
	config, err := sourceConfig(root)
	if err != nil {
		return "", err
	}
	context := build.Default
	context.GOOS, context.GOARCH = config["GOOS"], config["GOARCH"]
	context.CgoEnabled = config["CGO_ENABLED"] == "1"
	var flags []string
	if err := json.Unmarshal([]byte(config["GOFLAGS"]), &flags); err != nil {
		return "", err
	}
	for _, flag := range flags {
		if tags, ok := strings.CutPrefix(flag, "-tags="); ok {
			context.BuildTags = strings.FieldsFunc(tags, func(char rune) bool { return char == ',' || char == ' ' })
		}
	}
	// Host defaults affect source selection, rather than a portable contract's
	// identity. Declared module/toolchain versions remain source inputs.
	for _, key := range []string{"GOOS", "GOARCH", "CGO_ENABLED", "GOVERSION", "GO111MODULE", "GOAMD64", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM", "GOFIPS140"} {
		delete(config, key)
	}
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(config)
	_ = enc.Encode(inputs.Roots)
	_ = enc.Encode(inputs.Files)
	add := func(name string) error {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if os.IsNotExist(err) {
			_ = enc.Encode([]string{name, "missing"})
			return nil
		}
		if err != nil {
			return err
		}
		_ = enc.Encode([]string{name, fmt.Sprintf("%x", sha256.Sum256(body))})
		if strings.HasSuffix(name, ".go") {
			selected, err := context.MatchFile(filepath.Dir(filepath.Join(root, filepath.FromSlash(name))), filepath.Base(name))
			if err != nil {
				return err
			}
			_ = enc.Encode(selected)
		}
		return nil
	}
	for _, name := range inputs.Files {
		if err := add(name); err != nil {
			return "", err
		}
	}
	for _, name := range inputs.Roots {
		moduleRoot := filepath.Join(root, filepath.FromSlash(name))
		err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", ".hg", ".svn", "node_modules":
					return filepath.SkipDir
				}
				if path != moduleRoot {
					if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
						if err := add(relativeInput(root, filepath.Join(path, "go.mod"))); err != nil {
							return err
						}
						return filepath.SkipDir
					} else if !os.IsNotExist(err) {
						return err
					}
				}
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".stubs", ".c", ".h", ".cc", ".cpp", ".cxx", ".m", ".f", ".F", ".s", ".S", ".swig", ".swigcxx", ".syso":
				return add(relativeInput(root, path))
			}
			if entry.Name() == "go.mod" || entry.Name() == "go.sum" {
				return add(relativeInput(root, path))
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// Generate writes the client with the source-input manifest used by the fast
// freshness check. The standalone Generate function also embeds its contract hash.
func (c Collected) Generate(api string) (string, error) {
	text, err := Generate(c.Results, api, c.Module, c.Packages)
	if err != nil {
		return "", err
	}
	if c.Inputs == nil {
		return "", fmt.Errorf("contract source inputs were not collected")
	}
	inputs := *c.Inputs
	inputs.Contract = Fingerprint(c.Results)
	body, err := json.Marshal(inputs)
	if err != nil {
		return "", err
	}
	first, rest, _ := strings.Cut(text, "\n")
	return first + "\n" + inputsPrefix + string(body) + "\n" + rest, nil
}

// CheckFingerprint verifies the analyzed contract. Identical source inputs take
// the fast path; changed inputs are analyzed without generating or rewriting TS.
func CheckFingerprint(root string, generated []byte) (string, error) {
	var inputs SourceInputs
	found := false
	contract := ""
	for line := range strings.SplitSeq(string(generated), "\n") {
		if value, ok := strings.CutPrefix(line, "// contract-fingerprint: sha256:"); ok {
			contract = value
		}
		if value, ok := strings.CutPrefix(line, inputsPrefix); ok {
			if err := json.Unmarshal([]byte(value), &inputs); err != nil {
				return "", fmt.Errorf("invalid contract fingerprint: %w", err)
			}
			found = true
		}
	}
	if !found || inputs.Version != 2 || len(inputs.Roots) == 0 || len(inputs.Patterns) == 0 || len(contract) != 64 || contract != inputs.Contract {
		return "", fmt.Errorf("missing or unsupported contract fingerprint; regenerate with effectgo ts")
	}
	digest, err := inputs.current(root)
	if err != nil {
		return "", err
	}
	if digest == inputs.Digest {
		return contract, nil
	}
	collected, err := Collect(root, inputs.Patterns...)
	if err != nil {
		return "", fmt.Errorf("stale contract fingerprint; cannot analyze current contract: %w", err)
	}
	current := Fingerprint(collected.Results)
	if current != contract {
		return current, fmt.Errorf("stale contract fingerprint; regenerate with effectgo ts")
	}
	return current, nil
}
