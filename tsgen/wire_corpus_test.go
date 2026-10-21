package tsgen

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type wireCorpusCase struct {
	Name     string          `json:"name"`
	Payload  json.RawMessage `json:"payload"`
	GoStatus int             `json:"go_status"`
	Response map[string]any  `json:"response"`
}

// TestWireCorpus runs one generated Effect 4.0.4 client against the real rt/web router.
func TestWireCorpus(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	api, err := generate(t, "testdata/mod", "./ok/wirecorpus")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := filepath.Abs("testdata/mod")
	if err != nil {
		t.Fatal(err)
	}
	corpusBytes, err := os.ReadFile(filepath.Join(fixture, "ok/wirecorpus/cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus []wireCorpusCase
	if err := json.Unmarshal(corpusBytes, &corpus); err != nil || len(corpus) == 0 {
		t.Fatalf("wire corpus: %v", err)
	}
	serverBin := filepath.Join(t.TempDir(), "wirecorpus-server")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", serverBin, "./ok/wirecorpus/server")
	build.Dir = fixture
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build wire corpus server: %v\n%s", err, out)
	}
	server := exec.CommandContext(t.Context(), serverBin)
	server.Dir = fixture
	server.Stderr = os.Stderr
	stdout, err := server.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = server.Process.Kill()
		_ = server.Wait()
	}()
	scanner := bufio.NewScanner(stdout)
	startup := time.AfterFunc(15*time.Second, func() { _ = server.Process.Kill() })
	defer startup.Stop()
	if !scanner.Scan() {
		t.Fatalf("wire corpus server did not announce address: %v", scanner.Err())
	}
	startup.Stop()
	fields := strings.Fields(scanner.Text())
	if len(fields) != 3 || fields[0] != "READY" || fields[2] != fmt.Sprint(len(corpus)) {
		t.Fatalf("wire corpus server handshake = %q", scanner.Text())
	}
	baseURL := fields[1]
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		t.Fatalf("wire corpus server address = %q: %v", baseURL, err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	for _, tc := range corpus {
		resp, err := client.Post(baseURL+"/wire", "application/json", bytes.NewReader(tc.Payload))
		if err != nil {
			t.Fatalf("raw %s: %v", tc.Name, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			t.Fatalf("raw %s response: %v", tc.Name, readErr)
		}
		if resp.StatusCode != tc.GoStatus {
			t.Errorf("raw %s status = %d, want %d (%s)", tc.Name, resp.StatusCode, tc.GoStatus, body)
		}
		if resp.StatusCode == http.StatusOK {
			var out map[string]any
			if err := json.Unmarshal(body, &out); err != nil {
				t.Fatalf("raw %s output: %v", tc.Name, err)
			}
			if !reflect.DeepEqual(out, tc.Response) {
				t.Errorf("raw %s output = %#v, want %#v", tc.Name, out, tc.Response)
			}
		}
	}

	dir := t.TempDir()
	if err := os.Symlink(nm, filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"package.json":  `{"type":"module"}`,
		"api.gen.ts":    api,
		"cases.json":    string(corpusBytes),
		"tsconfig.json": `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","ESNext.Disposable"],"module":"NodeNext","moduleResolution":"NodeNext","strict":true,"types":[]},"include":["*.ts"]}`,
		"check.ts": fmt.Sprintf(`import { Effect, Schema } from "effect"
import { FetchHttpClient } from "effect/http"
import { HttpApiClient } from "effect/http-api"
import { Api } from "./api.gen.js"
const endpoint = Api.groups.wirecorpus.endpoints.echo
const schema = endpoint.payload.get("application/json")!.schemas[0] as Schema.Codec<unknown>
const cases = JSON.parse(%q) as Array<{name:string,payload:unknown,client_accepts:boolean,response:unknown}>
function canonical(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonical)
  if (value !== null && typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, canonical(item)]))
  return value
}
const program = Effect.gen(function*() {
  const client = yield* HttpApiClient.make(Api, { baseUrl: %q })
  for (const tc of cases) {
    if (!tc.client_accepts) {
      const error = yield* Effect.flip(client.wirecorpus.echo({ payload: tc.payload as never }))
      if (!Schema.isSchemaError(error)) throw new Error(tc.name + " escaped the client schema: " + String(error))
      continue
    }
    const encoded = Schema.encodeUnknownSync(schema)(tc.payload) as Record<string, unknown>
    if ("unknown" in encoded) throw new Error(tc.name + " retained an unknown request key")
    for (const key of ["name", "enabled", "count"]) {
      if (encoded[key] !== (tc.payload as Record<string, unknown>)[key]) throw new Error(tc.name + " changed " + key)
    }
    const value = yield* client.wirecorpus.echo({ payload: tc.payload as never })
    if (JSON.stringify(canonical(value)) !== JSON.stringify(canonical(tc.response))) throw new Error(tc.name + " response differs: " + JSON.stringify(value))
  }
})
await Effect.runPromise(program.pipe(Effect.provide(FetchHttpClient.layer)))
`, string(corpusBytes), baseURL),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{tsc, "-p", filepath.Join(dir, "tsconfig.json")}, {filepath.Join(dir, "check.js")}} {
		cmd := exec.CommandContext(t.Context(), node, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("wire corpus client: %v\n%s", err, out)
		}
	}
}
