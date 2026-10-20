package tsgen

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise the generated HttpApi decoder against actual untagged HTTP errors,
// including two distinct errors sharing status 503. Compilation alone cannot
// catch a schema that still requires the wire _tag.
func TestExternalErrorsDecode(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	api, err := generate(t, "testdata/mod", "./golden/external")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/api/external":
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"detail":"login required"}`))
		case "/disabled/api/external":
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"detail":{"error":"disabled","code":"MODULE_DISABLED"}}`))
		case "/maintenance/api/external":
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"detail":{"error":"maintenance","code":"MODULE_MAINTENANCE"}}`))
		case "/fraction/api/external":
			_, _ = w.Write([]byte(`{"name":"reference","remote_seq":1.25}`))
		default:
			_, _ = w.Write([]byte(`{"name":"reference","remote_seq":9007199254740993}`))
		}
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
import { Api, AuthenticationError, ModuleDisabled, ModuleMaintenance } from "./api.gen.js"
const baseUrl = ` + "`" + server.URL + "`" + `
for (const [path, expected] of [["auth", AuthenticationError], ["disabled", ModuleDisabled], ["maintenance", ModuleMaintenance]] as const) {
  const error = await Effect.runPromise(Effect.gen(function*() {
    const client = yield* HttpApiClient.make(Api, {baseUrl: baseUrl + "/" + path})
    return yield* Effect.flip(client.external.read({query: {}}))
  }).pipe(Effect.provide(FetchHttpClient.layer)))
  if (!(error instanceof expected) || error._tag !== expected.name) throw new Error("wrong decoded middleware error: " + path)
}
const result = await Effect.runPromise(Effect.gen(function*() {
  const client = yield* HttpApiClient.make(Api, {baseUrl})
  return yield* client.external.read({query: {seq: Number.MAX_SAFE_INTEGER + 2}})
}).pipe(Effect.provide(FetchHttpClient.layer)))
if (result.name !== "reference" || result.remote_seq <= Number.MAX_SAFE_INTEGER) throw new Error("64-bit response rejected or changed")
await Effect.runPromise(Effect.gen(function*() {
  const client = yield* HttpApiClient.make(Api, {baseUrl: baseUrl + "/fraction"})
  return yield* Effect.flip(client.external.read({query: {}}))
}).pipe(Effect.provide(FetchHttpClient.layer)))
console.log("decoded external 401/503 errors and numeric int64 beyond 2^53; rejected fractional integer")
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
			t.Fatalf("%v: %v\n%s", args, err, out)
		} else {
			t.Log(string(out))
		}
	}
}

func TestCookieBrowserTransport(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	api, err := generate(t, "testdata/mod", "./golden/cookies")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(nm, filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"package.json":  `{"type":"module"}`,
		"api.gen.ts":    api,
		"tsconfig.json": `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","ESNext.Disposable"],"module":"NodeNext","moduleResolution":"NodeNext","strict":true,"skipLibCheck":false,"types":[]},"include":["*.ts"]}`,
		"check.ts": `import { Effect, Layer } from "effect"
import { FetchHttpClient } from "effect/http"
import { HttpApiClient } from "effect/http-api"
import { Api, BrowserFetchHttpClient } from "./api.gen.js"

const observedFetch: typeof globalThis.fetch = async (_input, init) => {
  if (init?.credentials !== "include") throw new Error("cookie transport did not opt into credentials")
  if (new Headers(init?.headers).has("cookie")) throw new Error("cookie transport tried to set forbidden Cookie header")
  return new Response(JSON.stringify({ok: true}), {status: 200, headers: {"content-type": "application/json"}})
}

const transport = BrowserFetchHttpClient.pipe(
  Layer.provide(Layer.succeed(FetchHttpClient.Fetch, observedFetch)),
)
const result = await Effect.runPromise(Effect.gen(function*() {
  const client = yield* HttpApiClient.make(Api, {baseUrl: "https://example.test"})
  return yield* client.cookies.read({headers: {Authorization: "Bearer test"}})
}).pipe(Effect.provide(transport)))
if (!result.ok) throw new Error("unexpected cookie transport response")
console.log("browser cookie transport used credentials include without a Cookie header")
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
			t.Fatalf("%v: %v\n%s", args, err, out)
		} else {
			t.Log(string(out))
		}
	}
}
