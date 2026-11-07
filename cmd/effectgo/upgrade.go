package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

//go:embed module-path-map.json
var upgradeModuleMap []byte

const publicRuntimeModule = "github.com/effectgo/effectgo/rt"

type moduleRename struct {
	Since string `json:"since"`
	From  string `json:"from"`
	To    string `json:"to"`
}

type upgradeTools struct {
	client     *http.Client
	releaseURL string
	repository string
	platform   string
	out        io.Writer
	run        func(dir string, env []string, name string, args ...string) error
}

func runUpgrade(args []string) int {
	flags := flag.NewFlagSet("effectgo upgrade APP --to vX.Y.Z", flag.ContinueOnError)
	tag := flags.String("to", "", "release tag to install (vX.Y.Z)")
	// APP comes first, unlike package-pattern subcommands.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append(append([]string{}, args[1:]...), args[0])
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 || !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(*tag) || !semver.IsValid(*tag) {
		fmt.Fprintln(os.Stderr, "usage: effectgo upgrade APP --to vX.Y.Z")
		return 2
	}
	if semver.Compare(*tag, "v0.3.0") < 0 {
		fmt.Fprintln(os.Stderr, "effectgo upgrade: target must be v0.3.0 or newer (public Go module paths)")
		return 2
	}
	tools := upgradeTools{
		client:     &http.Client{Timeout: 5 * time.Minute},
		releaseURL: "https://github.com/effectgo/effectgo/releases/download",
		repository: "https://github.com/effectgo/effectgo.git",
		platform:   runtime.GOOS + "-" + runtime.GOARCH,
		out:        os.Stdout,
	}
	tools.run = func(dir string, env []string, name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return nil
	}
	if err := tools.upgrade(flags.Arg(0), *tag); err != nil {
		fmt.Fprintf(os.Stderr, "effectgo upgrade: %v\n", err)
		return 1
	}
	return 0
}

func (u upgradeTools) upgrade(app, tag string) error {
	if u.platform != "linux-arm64" && u.platform != "linux-amd64" {
		return fmt.Errorf("no release binary for %s; upgrade requires Linux arm64 or amd64", u.platform)
	}
	app, err := filepath.Abs(app)
	if err != nil {
		return err
	}
	old, err := readUpgradeLock(app)
	if err != nil {
		return err
	}
	oldRuntime, err := upgradeRuntimeFiles(old)
	if err != nil {
		return err
	}
	// Preserve local runtime edits rather than deleting them during the cutover.
	if err := verifyUpgradeRuntime(app, oldRuntime); err != nil {
		return err
	}
	files, err := upgradeFiles(app)
	if err != nil {
		return err
	}
	scripts, err := upgradeContractScripts(app)
	if err != nil {
		return err
	}
	clients, err := upgradeClients(app, files)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "effectgo-upgrade-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	fmt.Fprintf(u.out, "Upgrading %s to %s\n", app, tag)
	sums, err := u.download(u.releaseURL + "/" + tag + "/SHA256SUMS")
	if err != nil {
		return err
	}
	pinArgs := []string{app, "--release", tag}
	for _, platform := range []struct{ asset, lock string }{{"linux-arm64", "linux-arm64"}, {"linux-amd64", "linux-x64"}} {
		name := "effectgo-" + platform.asset
		expected, err := upgradeChecksum(sums, name)
		if err != nil {
			return err
		}
		data, err := u.download(u.releaseURL + "/" + tag + "/" + name)
		if err != nil {
			return err
		}
		if actual := fmt.Sprintf("%x", sha256.Sum256(data)); actual != expected {
			return fmt.Errorf("SHA256SUMS mismatch for %s: got %s, want %s", name, actual, expected)
		}
		binary := filepath.Join(work, name)
		if err := os.WriteFile(binary, data, 0o755); err != nil {
			return err
		}
		pinArgs = append(pinArgs, "--binary", platform.lock+"="+binary)
	}
	source := filepath.Join(work, "source")
	if err := u.run(work, nil, "git", "-c", "advice.detachedHead=false", "clone", "--quiet", "--depth=1", "--branch", tag, "--", u.repository, source); err != nil {
		return fmt.Errorf("obtain tagged source: %w", err)
	}
	version, err := os.ReadFile(filepath.Join(source, "VERSION"))
	if err != nil {
		return err
	}
	if "v"+strings.TrimSpace(string(version)) != tag {
		return fmt.Errorf("tagged source VERSION %q differs from %s", strings.TrimSpace(string(version)), tag)
	}
	renames, err := readUpgradeMap(source, tag)
	if err != nil {
		return err
	}
	if err := u.run(source, nil, "node", append([]string{filepath.Join(source, "scripts", "pin-source.mjs")}, pinArgs...)...); err != nil {
		return fmt.Errorf("repin consumer: %w", err)
	}
	modules, err := migrateUpgradeFiles(app, files, renames, tag)
	if err != nil {
		return err
	}
	if err := finishUpgradeRuntime(app, oldRuntime); err != nil {
		return err
	}
	// The repinner can replace scripts/effectgo.sh, so discover files again.
	files, err = upgradeFiles(app)
	if err != nil {
		return err
	}
	if err := reportUpgradeChecklist(u.out, app, files); err != nil {
		return err
	}
	env := []string{"GOWORK=off", "EFFECTGO_BIN=" + filepath.Join(work, "effectgo-"+u.platform), "EFFECTGO_DIR="}
	for _, module := range modules {
		if err := u.run(module, env, "go", "mod", "tidy"); err != nil {
			return err
		}
	}
	for _, script := range scripts {
		fmt.Fprintf(u.out, "Regenerating %s\n", script)
		if err := u.run(app, env, "bash", filepath.Join(app, script), "generate"); err != nil {
			return err
		}
	}
	files, err = upgradeFiles(app)
	if err != nil {
		return err
	}
	after, err := upgradeClients(app, files)
	if err != nil {
		return err
	}
	paths := maps.Clone(clients)
	maps.Copy(paths, after)
	changed := 0
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		if bytes.Equal(clients[path], after[path]) {
			continue
		}
		changed++
		beforeFile, afterFile := filepath.Join(work, "client-before"), filepath.Join(work, "client-after")
		if err := os.WriteFile(beforeFile, clients[path], 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(afterFile, after[path], 0o600); err != nil {
			return err
		}
		fmt.Fprintf(u.out, "Client diff: %s (+added/-removed lines below)\n", path)
		if err := u.run(app, nil, "git", "diff", "--no-index", "--numstat", "--", beforeFile, afterFile); err != nil {
			if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
				return err
			}
		}
	}
	fmt.Fprintf(u.out, "Upgraded to %s; tidied %d Go modules; regenerated %d contract scripts; %d clients changed. Review the application diff and the post-upgrade checklist above before committing.\n", tag, len(modules), len(scripts), changed)
	return nil
}

func (u upgradeTools) download(url string) ([]byte, error) {
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func upgradeChecksum(sums []byte, name string) (string, error) {
	found := ""
	for line := range strings.SplitSeq(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size || found != "" {
			return "", fmt.Errorf("invalid or duplicate SHA256SUMS entry for %s", name)
		}
		found = strings.ToLower(fields[0])
	}
	if found == "" {
		return "", fmt.Errorf("SHA256SUMS has no entry for %s", name)
	}
	return found, nil
}

func readUpgradeMap(source, tag string) ([]moduleRename, error) {
	data, err := os.ReadFile(filepath.Join(source, "cmd", "effectgo", "module-path-map.json"))
	if errors.Is(err, os.ErrNotExist) {
		data = upgradeModuleMap // Historical tags predate the cumulative map.
	} else if err != nil {
		return nil, err
	}
	var mapping struct {
		SchemaVersion int            `json:"schema_version"`
		Renames       []moduleRename `json:"renames"`
	}
	if err := json.Unmarshal(data, &mapping); err != nil {
		return nil, fmt.Errorf("module-path map: %w", err)
	}
	if mapping.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported module-path map schema %d", mapping.SchemaVersion)
	}
	var renames []moduleRename
	for _, rename := range mapping.Renames {
		if !semver.IsValid(rename.Since) || rename.From == "" || rename.To == "" {
			return nil, fmt.Errorf("invalid module-path map entry: %+v", rename)
		}
		if semver.Compare(rename.Since, tag) <= 0 {
			renames = append(renames, rename)
		}
	}
	return renames, nil
}

func upgradeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor", "third_party", ".cache", ".next", "dist", "build", "bin":
				return filepath.SkipDir
			}
		} else if entry.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func upgradeContractScripts(app string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(app, "scripts", "*-contract.sh"))
	if err != nil {
		return nil, err
	}
	aggregate := filepath.Join(app, "scripts", "api-contract.sh")
	if slices.Contains(paths, aggregate) {
		paths = []string{aggregate}
	}
	if len(paths) == 0 {
		return nil, errors.New("no scripts/*-contract.sh found; provide an application contract script accepting generate")
	}
	for i, path := range paths {
		paths[i], err = filepath.Rel(app, path)
		if err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func renamedUpgradePath(path string, renames []moduleRename) string {
	for _, rename := range renames {
		if path == rename.From || strings.HasPrefix(path, rename.From+"/") {
			path = rename.To + strings.TrimPrefix(path, rename.From)
		}
	}
	return path
}

func migrateUpgradeFiles(app string, files []string, renames []moduleRename, tag string) ([]string, error) {
	var modules []string
	for _, path := range files {
		if filepath.Base(path) == "go.mod" {
			modules = append(modules, filepath.Dir(path))
		}
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no Go modules found in %s", app)
	}
	usesRuntime := map[string]bool{}
	for _, path := range files {
		switch filepath.Ext(path) {
		case ".go", ".stubs", ".yaml", ".yml", ".toml", ".json":
		default:
			continue
		}
		if filepath.Base(path) == "effectgo.lock.json" || strings.HasPrefix(path, filepath.Join(app, "scripts", "vendor")+string(filepath.Separator)) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		updated := data
		switch filepath.Ext(path) {
		case ".go":
			set := token.NewFileSet()
			file, err := parser.ParseFile(set, path, data, parser.ImportsOnly)
			if err != nil {
				return nil, err
			}
			for _, imp := range slices.Backward(file.Imports) {
				old, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return nil, err
				}
				next := renamedUpgradePath(old, renames)
				if next == publicRuntimeModule || strings.HasPrefix(next, publicRuntimeModule+"/") {
					owner := ""
					for _, module := range modules {
						if strings.HasPrefix(path, module+string(filepath.Separator)) && len(module) > len(owner) {
							owner = module
						}
					}
					usesRuntime[owner] = true
				}
				if old != next {
					start, end := set.Position(imp.Path.Pos()).Offset, set.Position(imp.Path.End()).Offset
					updated = slices.Concat(updated[:start], []byte(strconv.Quote(next)), updated[end:])
				}
			}
		case ".stubs", ".yaml", ".yml", ".toml", ".json":
			// Keep application stubs and plugin configuration on the same paths.
			for _, rename := range renames {
				updated = bytes.ReplaceAll(updated, []byte(rename.From), []byte(rename.To))
			}
		default:
			continue
		}
		if !bytes.Equal(data, updated) {
			if err := os.WriteFile(path, updated, 0o644); err != nil {
				return nil, err
			}
		}
	}
	for _, dir := range modules {
		path := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		mod, err := modfile.Parse(path, data, nil)
		if err != nil {
			return nil, err
		}
		for _, require := range slices.Clone(mod.Require) {
			next := renamedUpgradePath(require.Mod.Path, renames)
			if next != require.Mod.Path || next == publicRuntimeModule || next == "github.com/effectgo/effectgo" {
				if err := mod.DropRequire(require.Mod.Path); err != nil {
					return nil, err
				}
				if err := mod.AddRequire(next, tag); err != nil {
					return nil, err
				}
			}
		}
		if usesRuntime[dir] {
			if err := mod.AddRequire(publicRuntimeModule, tag); err != nil {
				return nil, err
			}
		}
		for _, replace := range slices.Clone(mod.Replace) {
			old := renamedUpgradePath(replace.Old.Path, renames)
			next := renamedUpgradePath(replace.New.Path, renames)
			if old == publicRuntimeModule || old != replace.Old.Path || next != replace.New.Path {
				if err := mod.DropReplace(replace.Old.Path, replace.Old.Version); err != nil {
					return nil, err
				}
				if old != publicRuntimeModule {
					if err := mod.AddReplace(old, replace.Old.Version, next, replace.New.Version); err != nil {
						return nil, err
					}
				}
			}
		}
		mod.Cleanup()
		updated, err := mod.Format()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			return nil, err
		}
	}
	return modules, nil
}

func readUpgradeLock(app string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(filepath.Join(app, "effectgo.lock.json"))
	if err != nil {
		return nil, err
	}
	var lock map[string]json.RawMessage
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	if lock == nil {
		return nil, errors.New("effectgo.lock.json must be an object")
	}
	return lock, nil
}

func upgradeRuntimeFiles(lock map[string]json.RawMessage) (map[string]string, error) {
	files := map[string]string{}
	if data, ok := lock["runtime_files"]; ok {
		if err := json.Unmarshal(data, &files); err != nil {
			return nil, err
		}
	}
	if files == nil {
		files = map[string]string{}
	}
	for path := range files {
		if !filepath.IsLocal(path) {
			return nil, fmt.Errorf("unsafe runtime manifest path %q", path)
		}
	}
	return files, nil
}

func verifyUpgradeRuntime(app string, files map[string]string) error {
	root, err := os.OpenRoot(app)
	if err != nil {
		return err
	}
	defer root.Close()
	for path, expected := range files {
		data, err := root.ReadFile(filepath.Join("backend", "third_party", "effectgo-rt", path))
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != expected {
			return fmt.Errorf("runtime file %s has local changes; preserve them before upgrading", path)
		}
	}
	return nil
}

func finishUpgradeRuntime(app string, old map[string]string) error {
	lock, err := readUpgradeLock(app)
	if err != nil {
		return err
	}
	current, err := upgradeRuntimeFiles(lock)
	if err != nil {
		return err
	}
	maps.Copy(old, current)
	root, err := os.OpenRoot(app)
	if err != nil {
		return err
	}
	defer root.Close()
	base := filepath.Join("backend", "third_party", "effectgo-rt")
	old["SOURCE.json"] = ""
	for path := range old {
		if err := root.Remove(filepath.Join(base, path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// Only empty directories are removed; application-owned files survive.
		for dir := filepath.Dir(filepath.Join(base, path)); strings.HasPrefix(dir, base); dir = filepath.Dir(dir) {
			if err := root.Remove(dir); err != nil {
				break
			}
		}
	}
	delete(lock, "runtime_files")
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(app, "effectgo.lock.json"), append(data, '\n'), 0o644)
}

var upgradeRuntimeCall = regexp.MustCompile(`(effectgo-source\.mjs["']?[ \t]+)runtime\b`)

func reportUpgradeChecklist(out io.Writer, app string, files []string) error {
	fmt.Fprintln(out, "Post-upgrade checklist: review runtime snapshot references and replace obsolete effectgo-source.mjs runtime calls with tooling; matching consumer files are not edited automatically.")
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(app, path)
		if err != nil {
			return err
		}
		lineNumber := 0
		for line := range strings.SplitSeq(string(data), "\n") {
			lineNumber++
			if strings.Contains(line, "third_party/effectgo-rt") || upgradeRuntimeCall.MatchString(line) {
				fmt.Fprintf(out, "%s:%d: %s\n", filepath.ToSlash(relative), lineNumber, strings.TrimSpace(line))
			}
		}
	}
	return nil
}

func upgradeClients(app string, files []string) (map[string][]byte, error) {
	clients := map[string][]byte{}
	for _, path := range files {
		if filepath.Ext(path) != ".ts" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if bytes.HasPrefix(data, []byte("// Code generated by effectgo ts.")) {
			relative, err := filepath.Rel(app, path)
			if err != nil {
				return nil, err
			}
			clients[relative] = data
		}
	}
	return clients, nil
}
