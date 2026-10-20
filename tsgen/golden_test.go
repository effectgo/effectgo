package tsgen

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireTypeScript(t *testing.T) (node, tsc, nodeModules string) {
	t.Helper()
	mode := os.Getenv("EFFECTGO_STRICT_CROSS_LANGUAGE")
	strict := mode != "" && mode != "0"
	fail := func(format string, args ...any) {
		if strict {
			t.Fatalf("cross-language dependency missing: "+format, args...)
		}
		t.Skipf(format, args...)
	}
	var err error
	if node, err = exec.LookPath("node"); err != nil {
		fail("Node.js is not installed")
	}
	root, err := filepath.Abs("../examples/web/ts")
	if err != nil {
		t.Fatal(err)
	}
	nodeModules = filepath.Join(root, "node_modules")
	tsc = filepath.Join(nodeModules, "typescript", "bin", "tsc")
	if _, err := os.Stat(tsc); err != nil {
		fail("TypeScript is not installed at %s; run npm ci in examples/web/ts", tsc)
	}
	for _, dependency := range []struct {
		path, want string
	}{
		{filepath.Join(nodeModules, "effect", "package.json"), "4.0.4"},
		{filepath.Join(nodeModules, "@effect", "platform-node", "package.json"), "4.0.4"},
		{filepath.Join(nodeModules, "typescript", "package.json"), "7.0.2"},
	} {
		body, err := os.ReadFile(dependency.path)
		if err != nil {
			fail("dependency metadata is missing at %s; run npm ci in examples/web/ts", dependency.path)
		}
		var metadata struct{ Version string }
		if err := json.Unmarshal(body, &metadata); err != nil || metadata.Version != dependency.want {
			fail("%s is %q, want %s; run npm ci in examples/web/ts", dependency.path, metadata.Version, dependency.want)
		}
	}
	return node, tsc, nodeModules
}

func TestTypeScriptRequirementBehavior(t *testing.T) {
	if os.Getenv("EFFECTGO_TEST_REQUIRE_TYPESCRIPT") == "1" {
		requireTypeScript(t)
		t.Fatal("missing Node.js was accepted")
	}
	for _, strict := range []string{"0", "1"} {
		t.Run("strict="+strict, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestTypeScriptRequirementBehavior$", "-test.v")
			cmd.Env = append(os.Environ(), "EFFECTGO_TEST_REQUIRE_TYPESCRIPT=1", "EFFECTGO_STRICT_CROSS_LANGUAGE="+strict, "PATH="+t.TempDir())
			out, err := cmd.CombinedOutput()
			if strict == "1" {
				if err == nil || !strings.Contains(string(out), "cross-language dependency missing") {
					t.Fatalf("strict dependency check must fail: %v\n%s", err, out)
				}
			} else if err != nil || !strings.Contains(string(out), "--- SKIP:") {
				t.Fatalf("ordinary go test must skip without Node.js: %v\n%s", err, out)
			}
		})
	}
}

// tsconfigGolden compiles the generated mapping fixtures alone. The lib list is
// needed by effect's declarations when only the generated file is compiled.
const tsconfigGolden = `{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "ESNext.Disposable"],
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "strict": true,
    "noEmit": true,
    "skipLibCheck": false,
    "types": [],
    "verbatimModuleSyntax": true
  },
  "include": ["api.gen.ts"]
}
`

// TestGoldenCompiles generates every mapping fixture and compiles the current
// output against Effect as installed in examples/web/ts. Without Node.js or that
// dependency install, the test is skipped unless strict cross-language checks
// are enabled.
func TestGoldenCompiles(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	src, err := generate(t, "testdata/mod", "./golden/...")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"api.gen.ts":    src,
		"package.json":  `{"name":"tsgolden","private":true,"type":"module"}` + "\n",
		"tsconfig.json": tsconfigGolden,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(nm, filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), node, tsc, "--noEmit", "-p", filepath.Join(dir, "tsconfig.json"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tsc --noEmit on the generated mapping fixtures failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}
