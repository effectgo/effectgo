package tsgen

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runExternalFixture(t *testing.T, pkg string, args ...string) []byte {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", append([]string{"run", pkg}, args...)...)
	cmd.Dir = "testdata/mod"
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run external fixture %s: %v\n%s", pkg, err, out)
	}
	return out
}

func TestExternalErrorRegistrationConsistency(t *testing.T) {
	_, err := generate(t, "testdata/mod", "./bad/externalboundary")
	wantErr(t, err, "externalboundary.go:", "field EmbeddedError.Detail: embedded fields are not supported", "field InvalidRules.Detail: schema tag", `unknown rule "not-a-rule"`)
	for _, tc := range []struct{ mode, reason string }{
		{"embedded", "embedded fields are not supported"},
		{"rules", `unknown rule "not-a-rule"`},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			out := string(runExternalFixture(t, "./bad/externalboundary", tc.mode))
			for _, want := range []string{"web: GET /" + tc.mode + ": route option:", tc.reason} {
				if !strings.Contains(out, want) {
					t.Fatalf("registration diagnostic %q does not contain %q", out, want)
				}
			}
		})
	}
	if out := string(runExternalFixture(t, "./bad/externalboundary", "input")); !strings.Contains(out, "embedded fields are not supported") {
		t.Fatalf("schema input restriction changed: %s", out)
	}
}

func TestRouteOptionHints(t *testing.T) {
	_, err := generate(t, "testdata/mod", "./bad/externaloptions")
	wantErr(t, err, "externaloptions.go:", "route options must be direct calls", "unsupported route option")
}

func TestExternalErrorRuntimeWire(t *testing.T) {
	api, err := generate(t, "testdata/mod", "./ok/externalwire")
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(runExternalFixture(t, "./ok/externalwire"), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Status != http.StatusUnauthorized || strings.TrimSpace(string(wire.Body)) != `{"detail":"","count":"7"}` {
		t.Fatalf("external wire changed: %d %s", wire.Status, wire.Body)
	}
	node, tsc, nm := requireTypeScript(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/external-wire" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(wire.Status)
		_, _ = w.Write(wire.Body)
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.Symlink(nm, filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"package.json":  `{"type":"module"}`,
		"api.gen.ts":    api,
		"tsconfig.json": `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","ESNext.Disposable"],"module":"NodeNext","moduleResolution":"NodeNext","strict":true,"skipLibCheck":false,"types":[]},"include":["*.ts"]}`,
		"check.ts": `import { Effect } from "effect"
import { FetchHttpClient } from "effect/http"
import { HttpApiClient } from "effect/http-api"
import { Api, SupportedError } from "./api.gen.js"
const error = await Effect.runPromise(Effect.gen(function*() {
 const client = yield* HttpApiClient.make(Api, {baseUrl: ` + "`" + server.URL + "`" + `})
 return yield* Effect.flip(client.main.read())
}).pipe(Effect.provide(FetchHttpClient.layer)))
if (!(error instanceof SupportedError) || error._tag !== "SupportedError" || error.detail !== "" || error.count !== "7" || "note" in error) throw new Error("runtime external error failed output decoding")
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{tsc, "-p", filepath.Join(dir, "tsconfig.json")}, {filepath.Join(dir, "check.js")}} {
		cmd := exec.CommandContext(t.Context(), node, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("runtime external decoder: %v\n%s", err, out)
		}
	}
}
