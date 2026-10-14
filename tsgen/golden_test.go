package tsgen

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// update rewrites the golden file from the generator output. Review the diff
// line by line before committing it.
var update = flag.Bool("update", false, "rewrite testdata/golden/api.golden.ts from the generator")

// goldenFile is the generator output for every package of testdata/mod/golden.
const goldenFile = "testdata/golden/api.golden.ts"

// goldenPatterns are the packages of the golden output.
var goldenPatterns = []string{"./golden/..."}

// TestGolden: the output for the golden packages is the golden file. Each
// package covers rows of the mapping tables (see docs/design/framework.md).
func TestGolden(t *testing.T) {
	got, err := generate(t, "testdata/mod", goldenPatterns...)
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenFile, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("%v (run go test -run TestGolden -update to write it)", err)
	}
	if got != string(want) {
		t.Errorf("the output differs from %s; run go test -run TestGolden -update, then review the diff", goldenFile)
	}
}

// tsconfigGolden compiles the golden file alone. The lib list is needed by the
// effect declarations: DOM and ESNext.Disposable are used by effect's d.ts files
// when only the generated file is compiled.
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

// TestGoldenCompiles: the golden file compiles with tsc against effect@4.0.x as
// installed in examples/web/ts. Without node, or without that install, the test
// is skipped.
func TestGoldenCompiles(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; install Node.js to run the tsc check of the golden file")
	}
	nm, err := filepath.Abs("../examples/web/ts/node_modules")
	if err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join(nm, "typescript", "bin", "tsc")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("no typescript at %s: run npm install in examples/web/ts to run the tsc check", tsc)
	}
	src, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "tsgolden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"api.gen.ts":    string(src),
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
	cmd := exec.Command(node, tsc, "--noEmit", "-p", filepath.Join(dir, "tsconfig.json"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tsc --noEmit on the golden file failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}
