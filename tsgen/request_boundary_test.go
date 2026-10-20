package tsgen

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
)

// Exercise the generated encoder before HTTP and then the unmodified Fetch client.
func TestGeneratedJSONRequestBoundary(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	api, err := generate(t, "testdata/mod", "./ok/requestboundary")
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "POST" || r.URL.Path != "/body" && r.URL.Query().Get("limit") != "0" {
			http.Error(w, "changed request method or query type", http.StatusBadRequest)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var want string
		switch r.URL.Path {
		case "/strict/0":
			want = `{"name":"","count":0,"enabled":false,"nested":{"name":"nested"},"items":[{"name":"one"},{"name":"two","note":null}],"values":{"empty":"","nil":null},"opaque":{"nested":{"keep":0},"items":[{"keep":false},null,null,""]}}`
		case "/legacy/0":
			want = `{"extra":{"keep":false},"nested":{},"items":[{},null]}`
		case "/empty/0":
			want = `null`
		case "/body":
			want = `{"items":[{"name":"one"}]}`
		default:
			http.NotFound(w, r)
			return
		}
		var gotJSON, wantJSON any
		if err := json.Unmarshal(data, &gotJSON); err != nil {
			t.Errorf("%s invalid JSON body: %v: %s", r.URL.Path, err, data)
		} else if err := json.Unmarshal([]byte(want), &wantJSON); err != nil {
			t.Errorf("invalid expected body: %v", err)
		} else if !reflect.DeepEqual(gotJSON, wantJSON) {
			t.Errorf("%s body = %s, want %s", r.URL.Path, data, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.Symlink(nm, filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"package.json":  `{"type":"module"}`,
		"api.gen.ts":    api + "\nexport { omitUndefined as normalizeForTest }\n",
		"tsconfig.json": `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","ESNext.Disposable"],"module":"NodeNext","moduleResolution":"NodeNext","strict":true,"types":[]},"include":["*.ts"]}`,
		"check.ts": `import { Effect, Exit, Schema } from "effect"
import { FetchHttpClient } from "effect/http"
import { HttpApiClient, HttpApiEndpoint } from "effect/http-api"
import { Api, normalizeForTest } from "./api.gen.js"
const endpoints = Api.groups.requestboundary.endpoints
const schema = endpoints.strict.payload.get("application/json")!.schemas[0] as Schema.Codec<unknown>
const row = Object.freeze({name:"one",note:undefined})
const values = Object.freeze({omit:undefined,empty:"",nil:null})
const opaqueRow = Object.freeze({omit:undefined,keep:false})
const opaqueItems = Object.freeze([opaqueRow,null,null,""])
const opaque = Object.freeze({omit:undefined,nested:Object.freeze({omit:undefined,keep:0}),items:opaqueItems})
const payload = Object.freeze({name:"",count:0,enabled:false,note:undefined,nested:Object.freeze({name:"nested",note:undefined}),items:Object.freeze([row,Object.freeze({name:"two",note:null})]),values,opaque})
const encoded = Schema.encodeUnknownSync(schema)(payload) as typeof payload
if (Object.hasOwn(encoded,"note") || Object.hasOwn(encoded.nested,"note") || Object.hasOwn(encoded.items[0],"note") || Object.hasOwn(encoded.values,"omit") || Object.hasOwn(encoded.opaque,"omit") || Object.hasOwn(encoded.opaque.items[0] as object,"omit")) throw new Error("undefined object keys reached JSON serialization")
if (encoded.opaque.items.length !== 4 || !(1 in encoded.opaque.items) || encoded.opaque.items[1] !== null || encoded.opaque.items[2] !== null) throw new Error("array positions changed before JSON encoding")
if (encoded.name !== "" || encoded.count !== 0 || encoded.enabled !== false || encoded.items[1].note !== null || encoded.values.empty !== "" || encoded.values.nil !== null) throw new Error("defined falsy values changed")
if (!("note" in payload) || !("note" in row) || !("omit" in values) || !("omit" in opaqueRow) || payload.items[0] !== row || payload.opaque.items !== opaqueItems) throw new Error("caller input mutated")
const nullPrototype = Object.assign(Object.create(null), {omit:undefined,keep:false})
const prototypeKey = JSON.parse('{"__proto__":{"omit":null},"omit":null}') as Record<string,unknown>
prototypeKey.omit = undefined
const specials = [new Date("2026-10-20T00:00:00Z"),new Blob(["data"]),new File(["data"],"data.txt"),new Uint8Array([0,255]),new FormData(),new (class DTO { note = undefined })()]
const normalized = normalizeForTest({nullPrototype,prototypeKey,specials})
if ("omit" in normalized.nullPrototype || normalized.nullPrototype.keep !== false || Object.getPrototypeOf(normalized.nullPrototype) !== null) throw new Error("null-prototype object changed")
if (!Object.hasOwn(normalized.prototypeKey,"__proto__") || Object.getPrototypeOf(normalized.prototypeKey) !== Object.prototype) throw new Error("__proto__ became a prototype setter")
for (let i = 0; i < specials.length; i++) if (normalized.specials[i] !== specials[i]) throw new Error("special object was cloned or traversed")
const sparse: unknown[] = new Array(3)
sparse[2] = Object.freeze({omit:undefined})
const sparseNormalized = normalizeForTest(sparse)
if (sparseNormalized.length !== 3 || 0 in sparseNormalized || 1 in sparseNormalized || "omit" in (sparseNormalized[2] as object)) throw new Error("sparse array positions changed")
if (sparse.length !== 3 || 0 in sparse || 1 in sparse || !("omit" in (sparse[2] as object))) throw new Error("caller sparse array mutated")
const undefinedItems = Object.freeze([undefined,null])
if (normalizeForTest(undefinedItems) !== undefinedItems || !(0 in undefinedItems) || undefinedItems[0] !== undefined) throw new Error("undefined array element changed")
const baseline = HttpApiEndpoint.post("baseline", "/baseline", {payload:Schema.Unknown}).payload.get("application/json")!.schemas[0] as Schema.Codec<unknown>
for (const value of [...specials,undefinedItems,sparse]) {
 const original = Schema.encodeUnknownExit(baseline)(value)
 const result = Schema.encodeUnknownExit(schema)({...payload,opaque:value})
 if (Exit.isFailure(original) !== Exit.isFailure(result)) throw new Error("opaque JSON codec acceptance changed")
 if (Exit.isSuccess(original) && Exit.isSuccess(result) && JSON.stringify(original.value) !== JSON.stringify((result.value as {opaque:unknown}).opaque)) throw new Error("opaque JSON wire value changed")
}
const program = Effect.gen(function*() {
 const client = yield* HttpApiClient.make(Api,{baseUrl:` + "`" + server.URL + "`" + `})
 const request = {params:{id:0},query:{limit:0},payload:{...payload,values:{empty:"",nil:null}}}
 if (false) {
  // @ts-expect-error the JSON boundary preserves the required scalar input type
  client.requestboundary.strict({...request,payload:{...payload,count:"0"}})
  // @ts-expect-error required input fields remain required
  client.requestboundary.strict({...request,payload:{name:""}})
  // @ts-expect-error path numbers remain numbers
  client.requestboundary.strict({...request,params:{id:"0"}})
  // @ts-expect-error query numbers remain numbers
  client.requestboundary.strict({...request,query:{limit:"0"}})
 }
 if (!(yield* client.requestboundary.strict(request)).ok) throw new Error("strict HTTP request failed")
 if (!(yield* client.requestboundary.bodyOnly({payload:{items:[row]}})).ok) throw new Error("named JSON payload request failed")
 const legacy = {note:undefined,nested:{note:undefined},items:[{note:undefined},null],extra:{omit:undefined,keep:false}}
 if (!(yield* client.requestboundary.legacy({...request,payload:legacy})).ok) throw new Error("JSONBody HTTP request failed")
 if (!(yield* client.requestboundary.empty({...request,payload:null})).ok) throw new Error("JSONBody null changed")
 for (const bad of [
  {...request,payload:{...payload,count:-1}},
  {...request,payload:{...payload,count:undefined}},
  {...request,payload:{...payload,nested:{note:undefined}}},
  {...request,payload:{...payload,items:[row,undefined]}},
  {...request,payload:{...payload,items:[row,null]}},
  {...request,payload:{...payload,items:"invalid"}},
  {...request,payload:{...payload,opaque:undefinedItems}},
  {...request,params:{id:"0"}},
  {...request,query:{limit:"0"}},
 ]) {
  const error = yield* Effect.flip(client.requestboundary.strict(bad as never))
  if (!Schema.isSchemaError(error)) throw new Error("invalid payload escaped schema encoding")
 }
 const badLegacy = yield* Effect.flip(client.requestboundary.legacy({...request,payload:{enabled:"false"}} as never))
 if (!Schema.isSchemaError(badLegacy)) throw new Error("JSONBody scalar validation relaxed")
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
			t.Fatalf("generated JSON request boundary: %v\n%s", err, out)
		}
	}
	if got := requests.Load(); got != 4 {
		t.Errorf("server received %d requests, want 4; invalid inputs must fail before HTTP", got)
	}
}
