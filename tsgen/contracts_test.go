package tsgen

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenericInstantiationIdentity(t *testing.T) {
	c, err := Collect(filepath.Join("testdata", "mod"), "./ok/generics")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	pages, envelopes := 0, 0
	for _, result := range c.Results {
		for _, d := range result.Decls {
			if previous, ok := seen[d.Key]; ok && previous != d.Name {
				t.Fatalf("unstable name for %s", d.Key)
			}
			seen[d.Key] = d.Name
			if strings.Contains(d.Key, ".Page[") && strings.HasPrefix(d.Name, "Page_") {
				pages++
			}
			if strings.Contains(d.Key, ".Envelope[") && strings.HasPrefix(d.Name, "Envelope_") {
				envelopes++
			}
			if strings.HasSuffix(d.Key, ".First") && d.Name != "First" {
				t.Fatalf("non-generic name changed: %s", d.Name)
			}
		}
	}
	if pages != 2 || envelopes != 2 {
		t.Fatalf("instantiations collapsed: pages=%d envelopes=%d keys=%v", pages, envelopes, seen)
	}
	out, err := Generate(c.Results, "api", c.Module, c.Packages)
	if err != nil {
		t.Fatal(err)
	}
	for key, name := range seen {
		if strings.Contains(key, "[") && !strings.Contains(out, "export const "+name+" =") {
			t.Fatalf("missing instantiated declaration %s", key)
		}
	}
	other, err := generate(t, "testdata/mod", "./ok/generics")
	if err != nil {
		t.Fatal(err)
	}
	if out != other {
		t.Fatal("generic declaration names are nondeterministic")
	}
}

func TestSuccessMetadataRejected(t *testing.T) {
	_, err := generate(t, "testdata/mod", "./bad/metadata")
	wantErr(t, err, "SuccessStatus must be a constant integer", "invalid SuccessStatus", "a response body requires a 2xx", "NoContent requires SuccessStatus(204)", "RedirectResponse requires status", "SuccessStatus is declared more than once", "SuccessStatusRange requires a constant range", "SuccessStatuses requires Response[T]")
	wantErr(t, err, "ProxyResponse requires a single success status")
}

func TestBodyModeSchemas(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	api, err := generate(t, "testdata/mod", "./ok/bodymodes")
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
		"tsconfig.json": `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","ESNext.Disposable"],"module":"NodeNext","moduleResolution":"NodeNext","strict":true,"types":[]},"include":["*.ts"]}`,
		"check.ts": `import { Exit, Schema } from "effect"
import { Api, FormInput } from "./api.gen.js"
const endpoints = Api.groups.bodymodes.endpoints
function payload(name: keyof typeof endpoints, contentType = "application/json") {
 const entry = endpoints[name].payload.get(contentType)
 if (!entry || entry.schemas.length !== 1) throw new Error(name + ": missing native payload schema for " + contentType)
 return entry.schemas[0] as Schema.Codec<unknown>
}
function check(label: string, schema: Schema.ConstraintEncoder<unknown>, value: unknown, accepted = true) {
 if (Exit.isFailure(Schema.encodeUnknownExit(schema)(value)) === accepted) throw new Error(label + ": " + (accepted ? "rejected valid DTO" : "accepted invalid DTO"))
}
const strict = {name:"Ada",nested:{count:1},names:["one","two"]}
for (const name of ["legacy", "empty", "bestEffort"] as const) {
 const schema = payload(name)
 check(name + " complete", schema, {...strict,enabled:true})
 check(name + " root null", schema, null)
 check(name + " omitted fields", schema, {})
 check(name + " null fields", schema, {name:null,nested:null,names:null,enabled:null})
 check(name + " nested omission", schema, {nested:{}})
 check(name + " JSONBody ignores schema rules", schema, {nested:{count:0}})
 check(name + " nullable array elements", schema, {names:["one",null]})
 check(name + " wrong scalar", schema, {enabled:"true"}, false)
 check(name + " wrong array", schema, {names:"one"}, false)
 check(name + " wrong array element", schema, {names:[1]}, false)
 check(name + " empty body", schema, undefined, name !== "legacy")
 const decoded = Schema.decodeUnknownSync(schema)({extra:true,nested:{extra:"nested"}}) as {extra?: boolean,nested?: {extra?: string}}
 if (decoded.extra !== true || decoded.nested?.extra !== "nested") throw new Error(name + ": JSONBody discarded unknown keys")
}
if (endpoints.remove.method !== "DELETE") throw new Error("remove lost its DELETE method")
for (const name of ["strict", "remove"] as const) {
 const schema = payload(name)
 check(name + " required fields", schema, strict)
 check(name + " nullable optional boolean", schema, {...strict,enabled:null})
 check(name + " optional boolean", schema, {...strict,enabled:false})
 check(name + " missing body", schema, undefined, false)
 check(name + " null body", schema, null, false)
 check(name + " missing required fields", schema, {}, false)
 check(name + " nested validation", schema, {...strict,nested:{count:0}}, false)
 check(name + " null array", schema, {...strict,names:null}, false)
 check(name + " null array element", schema, {...strict,names:[null]}, false)
 check(name + " wrong boolean", schema, {...strict,enabled:"true"}, false)
}
const multipart = payload("form", "multipart/form-data")
const file = new File([new Uint8Array([0,255])], "data.bin")
const form = {file,commit_token:"token",names:["one","two"]}
check("multipart omitted optionals", multipart, form)
check("multipart scalar values", multipart, {...form,enabled:false,count:0,note:"",attachment:file})
check("multipart empty array", multipart, {...form,names:[]})
for (const key of ["enabled", "count", "note", "attachment"] as const) {
 check("multipart null " + key, multipart, {...form,[key]:null}, false)
}
check("multipart null required file", multipart, {...form,file:null}, false)
check("multipart missing file", multipart, {commit_token:"token",names:[]}, false)
check("multipart blob is not File", multipart, {...form,file:new Blob(["data"])}, false)
check("multipart string is not File", multipart, {...form,file:"data.bin"}, false)
check("multipart missing token", multipart, {file,names:[]}, false)
check("multipart missing array", multipart, {file,commit_token:"token"}, false)
check("multipart scalar is not array", multipart, {...form,names:"one"}, false)
check("multipart null array", multipart, {...form,names:null}, false)
check("multipart null array element", multipart, {...form,names:[null]}, false)
check("multipart wrong array element", multipart, {...form,names:[1]}, false)
check("JSON declaration retains nullable scalars", FormInput, {commit_token:"token",names:[],enabled:null,count:null,note:null})
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
			t.Fatalf("generated body mode schemas: %v\n%s", err, out)
		}
	}
}

// Native Effect status annotations and binary codecs must survive real HTTP decoding.
func TestGeneratedResponseContracts(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	api, err := generate(t, "testdata/mod", "./ok/responses")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"HttpApiSchema.status(201)", "HttpApiSchema.status(202)", "Schema.Uint8Array.pipe(HttpApiSchema.asUint8Array())", "HttpApiSchema.status(206)", "HttpApiSchema.Empty(304)", "HttpApiSchema.status(299)", "HttpApiSchema.Empty(205)", `HttpApiSchema.asText({ contentType: "text/html" })`} {
		if !strings.Contains(api, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(api, "io.ReadCloser") || strings.Contains(api, "export const FileResponse") || strings.Contains(api, "export const Response_") {
		t.Fatal("response metadata was serialized as a JSON schema")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/inherited":
			type Batch struct {
				ID      int      `json:"id"`
				Title   chan int `json:"title"`
				Ignored chan int `json:"-"`
				hidden  chan int
			}
			type BatchExtra struct {
				Note     *string  `json:"note"`
				Labels   []string `json:"labels,omitempty"`
				Required []string `json:"required"`
			}
			type Row struct {
				SKU  string `json:"sku"`
				Meta struct {
					Active bool `json:"active"`
				} `json:"meta"`
			}
			type Detail struct {
				Batch
				*BatchExtra
				Title string `json:"title"`
				Rows  []Row  `json:"rows"`
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode([]Detail{
				{Batch: Batch{ID: 1}, Title: "nil", Rows: nil},
				{Batch: Batch{ID: 2}, BatchExtra: &BatchExtra{}, Title: "full", Rows: []Row{{SKU: "sku"}}},
			}); err != nil {
				t.Errorf("encode inherited output: %v", err)
			}
		case "/created":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"name":"created"}`))
		case "/accepted":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			_, _ = w.Write([]byte(`{"name":"accepted"}`))
		case "/file":
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="label.pdf"`)
			_, _ = w.Write([]byte{0, 255, 1, 2})
		case "/conditional":
			w.Header().Set("Content-Type", "application/pdf")
			http.ServeContent(w, r, "label.pdf", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), bytes.NewReader([]byte{0, 255, 1, 2}))
		case "/stream":
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(299)
			_, _ = w.Write([]byte{3, 0, 255})
		case "/empty":
			w.WriteHeader(204)
		case "/dynamic":
			w.WriteHeader(205)
		case "/redirect":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Location", "/destination")
			w.WriteHeader(302)
			_, _ = w.Write([]byte("<a>redirect</a>"))
		default:
			http.NotFound(w, r)
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
		"tsconfig.json": `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","ESNext.Disposable"],"module":"NodeNext","moduleResolution":"NodeNext","strict":true,"types":[]},"include":["*.ts"]}`,
		"check.ts": `import { Effect, Layer } from "effect"
import { FetchHttpClient } from "effect/http"
import { HttpApiClient } from "effect/http-api"
import { Api } from "./api.gen.js"
const program = Effect.gen(function*() {
 const client = yield* HttpApiClient.make(Api,{baseUrl:` + "`" + server.URL + "`" + `})
 if ((yield* client.responses.created()).name !== "created") throw new Error("201 decode failed")
 if ((yield* client.responses.accepted()).name !== "accepted") throw new Error("202 decode failed")
 const [bytes,response] = yield* client.responses.file({responseMode:"decoded-and-response"})
 if (!(bytes instanceof Uint8Array) || bytes[1] !== 255 || response.headers["content-disposition"] !== 'attachment; filename="label.pdf"') throw new Error("binary decode failed")
 const [range,rangeResponse] = yield* client.responses.conditional({headers:{Range:"bytes=1-2"},responseMode:"decoded-and-response"})
 if (!(range instanceof Uint8Array) || range.length !== 2 || range[0] !== 255 || rangeResponse.status !== 206 || rangeResponse.headers["content-range"] !== "bytes 1-2/4") throw new Error("native 206 decode failed")
 const [unchanged,conditionalResponse] = yield* client.responses.conditional({headers:{"If-Modified-Since":"Wed, 01 Jan 2020 00:00:00 GMT"},responseMode:"decoded-and-response"})
 if (unchanged !== undefined || conditionalResponse.status !== 304) throw new Error("native 304 decode failed")
 const streamed = yield* client.responses.stream()
 if (!(streamed instanceof Uint8Array) || streamed[2] !== 255) throw new Error("dynamic binary status failed")
 if ((yield* client.responses.empty()) !== undefined) throw new Error("204 decode failed")
 if ((yield* client.responses.dynamic()) !== undefined) throw new Error("205 decode failed")
 if ((yield* client.responses.redirect()) !== "<a>redirect</a>") throw new Error("302 HTML decode failed")
 const details = yield* client.responses.inherited()
 if (details === null || details.length !== 2) throw new Error("inherited array decode failed")
 const [empty,full] = details
 if (empty.id !== 1 || empty.title !== "nil" || empty.rows !== null || "note" in empty || "required" in empty) throw new Error("nil embedded pointer changed JSON shape")
 if (full.id !== 2 || full.note !== null || full.required !== null || "labels" in full || full.rows?.[0].sku !== "sku" || full.rows[0].meta.active !== false) throw new Error("inherited anonymous output decode failed")
 if ("Ignored" in full || "hidden" in full || "Batch" in full) throw new Error("ignored or embedded field leaked")
})
await Effect.runPromise(program.pipe(Effect.provide(FetchHttpClient.layer.pipe(Layer.provide(Layer.succeed(FetchHttpClient.RequestInit,{redirect:"manual"}))))))
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
			t.Fatalf("generated response client: %v\n%s", err, out)
		}
	}
}
