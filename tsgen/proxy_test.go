package tsgen

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestGeneratedProxyResponseContracts(t *testing.T) {
	c, err := Collect(filepath.Join("testdata", "mod"), "./ok/proxy")
	if err != nil {
		t.Fatal(err)
	}
	routes := 0
	for _, result := range c.Results {
		for _, route := range result.Routes {
			routes++
			if route.Out == nil || route.Out.Root.Name != "ProxySuccess" || route.ProxyFailure == nil || route.ProxyFailure.Root.Name != "ProxyFailure" {
				t.Fatalf("proxy generic arguments were not separated: %+v", route)
			}
			status := http.StatusOK
			if route.Name == "created" {
				status = http.StatusCreated
			}
			if route.SuccessStatus != status || !slices.Equal(route.SuccessStatuses, []int{status}) {
				t.Fatalf("proxy success statuses widened: %+v", route)
			}
			for _, failure := range route.Errors {
				if failure.Name == "ProxyFailure" {
					t.Fatal("provider failure was assigned a constant typed error status")
				}
			}
		}
	}
	if routes != 2 {
		t.Fatalf("expected both aliased and direct ProxyResponse routes, got %d", routes)
	}
	api, err := Generate(c.Results, "api", c.Module, c.Packages)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"export const ProxyFailure = Schema.Struct({", "success: ProxySuccess,", "success: ProxySuccess.pipe(HttpApiSchema.status(201)),"} {
		if !strings.Contains(api, want) {
			t.Fatalf("missing %q in generated proxy schemas", want)
		}
	}
	if strings.Contains(api, "export const ProxyResponse_") || strings.Contains(api, "export class ProxyFailure") {
		t.Fatal("proxy metadata or provider failure was emitted as a fixed error contract")
	}
	node, tsc, nm := requireTypeScript(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/proxy-created" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"created"}`))
			return
		}
		status, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/proxy/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if status == 0 {
			_, _ = w.Write([]byte(`{"name":123}`))
			return
		}
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"name":"ok"}`))
			return
		}
		w.Header().Set("X-Provider-Status", strconv.Itoa(status))
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `{"message":"provider failure","code":%d}`, status)
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.Symlink(nm, filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"package.json":  `{"type":"module"}`,
		"api.gen.ts":    api,
		"tsconfig.json": `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","ESNext.Disposable"],"module":"NodeNext","moduleResolution":"NodeNext","strict":true,"types":[]},"include":["*.ts"]}`,
		"check.ts": `import { Effect, Exit, Schema } from "effect"
import { FetchHttpClient } from "effect/http"
import { HttpApiClient } from "effect/http-api"
import { Api, ProxyFailure, ProxySuccess } from "./api.gen.js"
const program = Effect.gen(function*() {
 const client = yield* HttpApiClient.make(Api,{baseUrl:` + "`" + server.URL + "`" + `})
 const ok = yield* client.proxy.proxy({params:{status:200}})
 const name: string = ok.name
 // @ts-expect-error generated client success remains a strict string field
 const invalidName: number = ok.name
 // @ts-expect-error standalone success schema rejects a numeric name at compile time
 const invalidBody: Schema.Schema.Type<typeof ProxySuccess> = {name:123}
 if (name !== "ok") throw new Error("proxy 200 decode failed")
 if ((yield* client.proxy.created()).name !== "created") throw new Error("proxy default 201 decode failed")
 const malformed = yield* Effect.exit(client.proxy.proxy({params:{status:0}}))
 if (!Exit.isFailure(malformed)) throw new Error("malformed proxy 200 passed generated success decoding")
 const rawMalformed = yield* client.proxy.proxy({params:{status:0},responseMode:"response-only"})
 const malformedBody = yield* rawMalformed.json
 if (!Exit.isFailure(Schema.decodeUnknownExit(ProxySuccess)(malformedBody))) throw new Error("manual generated success decoder accepted malformed 200")
 const rawSuccess = yield* client.proxy.proxy({params:{status:200},responseMode:"response-only"})
 const success = Schema.decodeUnknownSync(ProxySuccess)(yield* rawSuccess.json)
 if (rawSuccess.status !== 200 || success.name !== "ok") throw new Error("manual strict success decoding failed")
 for (const status of [206,429,507]) {
  const response = yield* client.proxy.proxy({params:{status},responseMode:"response-only"})
  const body = yield* response.json
  const failure = Schema.decodeUnknownSync(ProxyFailure)(body)
  if (response.status !== status || response.headers["x-provider-status"] !== String(status) || failure.code !== status || failure.message !== "provider failure") throw new Error("provider status/body/header changed")
  if (!Exit.isFailure(Schema.decodeUnknownExit(ProxySuccess)(body))) throw new Error("provider failure matched strict success schema")
 }
 const decodedPartial = yield* Effect.exit(client.proxy.proxy({params:{status:206}}))
 if (!Exit.isFailure(decodedPartial)) throw new Error("provider 206 was added as a success alternative")
})
await Effect.runPromise(program.pipe(Effect.provide(FetchHttpClient.layer)))
`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{tsc, "-p", filepath.Join(dir, "tsconfig.json")}, {filepath.Join(dir, "check.js")}} {
		cmd := exec.CommandContext(t.Context(), node, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generated proxy response client: %v\n%s", err, out)
		}
	}
}
