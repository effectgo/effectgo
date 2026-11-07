package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestUpgradeConsumer(t *testing.T) {
	for _, legacyPin := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy target pin=%t", legacyPin), func(t *testing.T) {
			app := upgradeFixture(t)
			// An aggregate script wins over individual scripts, avoiding duplicate generation.
			if !legacyPin {
				writeUpgradeFixture(t, filepath.Join(app, "scripts/api-contract.sh"), readUpgradeFixture(t, filepath.Join(app, "scripts/reference-contract.sh")))
			}
			preserved := map[string]string{
				"Dockerfile":                    "FROM golang:1.26\nCOPY backend/third_party/effectgo-rt /app/rt\n",
				"backend/Dockerfile":            "COPY third_party/effectgo-rt /app/rt\n",
				"docs/upgrade.md":               "Remove backend/third_party/effectgo-rt.\n",
				"Taskfile.yml":                  "tasks:\n  runtime:\n    cmds:\n      - node scripts/effectgo-source.mjs runtime\n",
				"scripts/reference-contract.sh": readUpgradeFixture(t, filepath.Join(app, "scripts/reference-contract.sh")),
			}
			for path, content := range preserved {
				writeUpgradeFixture(t, filepath.Join(app, path), content)
			}
			assets := upgradeTestAssets()
			server := upgradeTestServer(t, assets)
			var output bytes.Buffer
			var commands []string
			var verifiedBinary string
			tools := upgradeTools{
				client: server.Client(), releaseURL: server.URL, repository: "https://example.invalid/effectgo.git",
				platform: "linux-arm64", out: &output,
			}
			tools.run = func(dir string, env []string, name string, args ...string) error {
				commands = append(commands, name+" "+strings.Join(args, " "))
				switch name {
				case "git":
					if args[0] == "diff" {
						cmd := exec.Command("git", args...)
						cmd.Stdout, cmd.Stderr = &output, &output
						return cmd.Run()
					}
					if !slices.Equal(args[:3], []string{"-c", "advice.detachedHead=false", "clone"}) || !slices.Contains(args, "v0.4.0") || args[len(args)-2] != tools.repository {
						t.Fatalf("source was not cloned at the requested tag: %v", args)
					}
					source := args[len(args)-1]
					writeUpgradeFixture(t, filepath.Join(source, "VERSION"), "0.4.0\n")
					writeUpgradeFixture(t, filepath.Join(source, "scripts/pin-source.mjs"), "// stubbed source pin tool\n")
					if !legacyPin {
						writeUpgradeFixture(t, filepath.Join(source, "cmd/effectgo/module-path-map.json"), string(upgradeModuleMap))
					}
				case "node":
					if len(args) != 8 || args[1] != app || args[2] != "--release" || args[3] != "v0.4.0" || filepath.Base(args[0]) != "pin-source.mjs" {
						t.Fatalf("wrong source repin invocation: %v", args)
					}
					binaries := map[string]string{}
					for _, arg := range []string{args[5], args[7]} {
						platform, path, ok := strings.Cut(arg, "=")
						if !ok {
							t.Fatalf("invalid binary pin: %q", arg)
						}
						data := readUpgradeFixture(t, path)
						asset := "effectgo-" + strings.ReplaceAll(platform, "x64", "amd64")
						if data != assets[asset] {
							t.Fatalf("repinned unverified binary %s", platform)
						}
						binaries[platform] = fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
						if platform == "linux-arm64" {
							verifiedBinary = path
						}
					}
					lock := map[string]any{"effectgo_version": "0.4.0", "release_tag": "v0.4.0", "snapshot_kind": "release", "binaries": binaries}
					if legacyPin {
						// Legacy pin tools replace the runtime inventory. Cleanup must
						// remove both the previous inventory and newly copied files.
						writeUpgradeFixture(t, filepath.Join(app, "backend/third_party/effectgo-rt/web/new.go"), "package web\n")
						lock["runtime_files"] = map[string]string{"web/new.go": "stub pin digest"}
					}
					data, err := json.Marshal(lock)
					if err != nil {
						t.Fatal(err)
					}
					writeUpgradeFixture(t, filepath.Join(app, "effectgo.lock.json"), string(data))
				case "go":
					if dir != filepath.Join(app, "backend") || !slices.Equal(args, []string{"mod", "tidy"}) || !slices.Contains(env, "GOWORK=off") {
						t.Fatalf("wrong module tidy: %s %v %v", dir, env, args)
					}
					// Import/go.mod migrations are real; only dependency fetching is stubbed.
					assertUpgradedModule(t, app)
					writeUpgradeFixture(t, filepath.Join(dir, "go.sum"), "// fixture tidy completed\n")
				case "bash":
					want := "reference-contract.sh"
					if !legacyPin {
						want = "api-contract.sh"
					}
					if dir != app || filepath.Base(args[0]) != want || len(args) != 2 || args[1] != "generate" || !slices.Contains(env, "EFFECTGO_BIN="+verifiedBinary) || !slices.Contains(env, "EFFECTGO_DIR=") {
						t.Fatalf("wrong regeneration: %s %v %v", dir, env, args)
					}
					if script := readUpgradeFixture(t, args[0]); !strings.Contains(script, "mjs\" runtime") {
						t.Fatalf("consumer runtime verification was edited automatically: %s", script)
					}
					writeUpgradeFixture(t, filepath.Join(app, "frontend/client.gen.ts"), "// Code generated by effectgo ts. DO NOT EDIT.\nexport const version = \"v0.4.0\"\n")
				default:
					t.Fatalf("unexpected external tool: %s", name)
				}
				return nil
			}
			if err := tools.upgrade(app, "v0.4.0"); err != nil {
				t.Fatal(err)
			}
			assertUpgradedModule(t, app)
			lock, err := readUpgradeLock(app)
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := lock["runtime_files"]; exists {
				t.Fatal("runtime manifest survived the public-module cutover")
			}
			for _, path := range []string{"go.mod", "schema/schema.go", "SOURCE.json", "web/new.go"} {
				if _, err := os.Stat(filepath.Join(app, "backend/third_party/effectgo-rt", path)); !os.IsNotExist(err) {
					t.Errorf("old managed runtime file survived: %s (%v)", path, err)
				}
			}
			if got := readUpgradeFixture(t, filepath.Join(app, "backend/third_party/effectgo-rt/local.txt")); got != "application owned\n" {
				t.Fatalf("application-owned runtime file changed: %q", got)
			}
			if !strings.Contains(output.String(), "frontend/client.gen.ts") || !strings.Contains(output.String(), "1 clients changed") || !strings.Contains(output.String(), "regenerated 1 contract scripts") {
				t.Fatalf("missing client diff summary: %s", &output)
			}
			for path, content := range preserved {
				if got := readUpgradeFixture(t, filepath.Join(app, path)); got != content {
					t.Errorf("consumer file %s changed: %q", path, got)
				}
			}
			checklist := []string{
				"Post-upgrade checklist:",
				"Dockerfile:2: COPY backend/third_party/effectgo-rt /app/rt",
				"Taskfile.yml:4: - node scripts/effectgo-source.mjs runtime",
				"backend/Dockerfile:1: COPY third_party/effectgo-rt /app/rt",
				"docs/upgrade.md:1: Remove backend/third_party/effectgo-rt.",
				"scripts/reference-contract.sh:4: node",
			}
			previous := -1
			for _, hit := range checklist {
				index := strings.Index(output.String(), hit)
				if index <= previous {
					t.Fatalf("missing or unordered checklist hit %q: %s", hit, &output)
				}
				previous = index
			}
			if previous >= strings.Index(output.String(), "Regenerating ") {
				t.Fatalf("checklist must precede regeneration: %s", &output)
			}
			if len(commands) != 5 {
				t.Fatalf("unexpected external commands: %v", commands)
			}
		})
	}
}

func TestUpgradeChecklist(t *testing.T) {
	app := t.TempDir()
	reference := "COPY backend/third_party/effectgo-rt ./third_party/effectgo-rt\n"
	want := []string{"Dockerfile", "Taskfile.yml", "backend/Dockerfile", "docs/upgrade.md", "ops/deploy"}
	for _, path := range want {
		writeUpgradeFixture(t, filepath.Join(app, path), reference)
	}
	for _, path := range []string{
		"effectgo.lock.json", "scripts/effectgo.sh", "scripts/effectgo-source.mjs",
		"scripts/vendor/effectgo/verify-source.mjs", "frontend/client.gen.ts",
		".git/config", ".hg/store/data", ".svn/entries", "node_modules/pkg/index.js",
		"backend/vendor/pkg/source.go", "third_party/pkg/source.go", ".cache/output",
		".next/output", "dist/output", "build/output", "bin/output",
	} {
		content := reference
		if path == "frontend/client.gen.ts" {
			content = "// Code generated by effectgo ts. DO NOT EDIT.\n" + reference
		}
		writeUpgradeFixture(t, filepath.Join(app, path), content)
	}
	writeUpgradeFixture(t, filepath.Join(app, "image.dat"), "\x00"+reference)
	writeUpgradeFixture(t, filepath.Join(app, "binary.dat"), "\xff"+reference)
	files, err := upgradeFiles(app)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := reportUpgradeChecklist(&output, app, files); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	var expected []string
	for _, path := range want {
		expected = append(expected, path+":1: "+strings.TrimSpace(reference))
	}
	if !slices.Equal(lines[1:], expected) {
		t.Fatalf("checklist entries:\n%s\nwant: %v", &output, expected)
	}
}

func TestUpgradeRejectsInvalidInputsBeforeRepinning(t *testing.T) {
	for _, failure := range []string{"tampered binary", "missing checksum", "modified runtime"} {
		t.Run(failure, func(t *testing.T) {
			app := upgradeFixture(t)
			before := readUpgradeFixture(t, filepath.Join(app, "effectgo.lock.json"))
			assets := upgradeTestAssets()
			switch failure {
			case "tampered binary":
				assets["effectgo-linux-arm64"] = "tampered"
			case "missing checksum":
				assets["SHA256SUMS"] = ""
			case "modified runtime":
				writeUpgradeFixture(t, filepath.Join(app, "backend/third_party/effectgo-rt/schema/schema.go"), "package locallymodified\n")
			}
			server := upgradeTestServer(t, assets)
			var output bytes.Buffer
			tools := upgradeTools{client: server.Client(), releaseURL: server.URL, platform: "linux-arm64", out: &output}
			tools.run = func(string, []string, string, ...string) error {
				t.Fatal("tools ran before input verification completed")
				return nil
			}
			if err := tools.upgrade(app, "v0.4.0"); err == nil {
				t.Fatal("invalid upgrade inputs were accepted")
			}
			if got := readUpgradeFixture(t, filepath.Join(app, "effectgo.lock.json")); got != before {
				t.Fatal("consumer was repinned before validation")
			}
		})
	}
}

func upgradeFixture(t *testing.T) string {
	t.Helper()
	app := t.TempDir()
	if err := os.CopyFS(app, os.DirFS("testdata/upgrade")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	base := filepath.Join(app, "backend/third_party/effectgo-rt")
	if err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		files[relative] = fmt.Sprintf("%x", sha256.Sum256([]byte(readUpgradeFixture(t, path))))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	lock, err := json.Marshal(map[string]any{"effectgo_version": "0.2.0", "runtime_files": files, "binaries": map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	writeUpgradeFixture(t, filepath.Join(app, "effectgo.lock.json"), string(lock))
	writeUpgradeFixture(t, filepath.Join(base, "SOURCE.json"), string(lock))
	writeUpgradeFixture(t, filepath.Join(base, "local.txt"), "application owned\n")
	return app
}

func assertUpgradedModule(t *testing.T, app string) {
	t.Helper()
	goSource := readUpgradeFixture(t, filepath.Join(app, "backend/routes.go"))
	if strings.Contains(goSource, "faintghost") || !strings.Contains(goSource, `web "github.com/effectgo/effectgo/rt/web"`) || !strings.Contains(goSource, `"github.com/effectgo/effectgo/rt/schema"`) {
		t.Fatalf("imports were not migrated: %s", goSource)
	}
	data := readUpgradeFixture(t, filepath.Join(app, "backend/go.mod"))
	mod, err := modfile.Parse("go.mod", []byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(mod.Require) != 1 || mod.Require[0].Mod.Path != publicRuntimeModule || mod.Require[0].Mod.Version != "v0.4.0" || len(mod.Replace) != 0 {
		t.Fatalf("public module require/replace migration failed: %s", data)
	}
	if stub := readUpgradeFixture(t, filepath.Join(app, "backend/effectgo.stubs")); strings.Contains(stub, "faintghost") || !strings.Contains(stub, publicRuntimeModule+"/web.Get") {
		t.Fatalf("stub paths were not migrated: %s", stub)
	}
}

func upgradeTestAssets() map[string]string {
	assets := map[string]string{"effectgo-linux-arm64": "fixture arm64 binary", "effectgo-linux-amd64": "fixture amd64 binary"}
	for _, name := range []string{"effectgo-linux-arm64", "effectgo-linux-amd64"} {
		assets["SHA256SUMS"] += fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(assets[name])), name)
	}
	return assets
}

func upgradeTestServer(t *testing.T, assets map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutPrefix(r.URL.Path, "/v0.4.0/")
		data, exists := assets[name]
		if !ok || !exists {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, data)
	}))
	t.Cleanup(server.Close)
	return server
}

func writeUpgradeFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readUpgradeFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
