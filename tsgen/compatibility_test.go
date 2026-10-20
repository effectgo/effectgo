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

func TestCompatibilityJSONClient(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	api, err := generate(t, "testdata/mod", "./ok/compatibility")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Schema.suspend", "Schema.Codec<JsonBody", "count: Schema.optionalKey(Schema.NullOr(Schema.String))", "enabled: Schema.optionalKey(Schema.NullOr(Schema.String))", "name: Schema.optionalKey(Schema.NullOr(Schema.String))", "ratio: Schema.optionalKey(Schema.NullOr(Schema.String))", "token: Schema.optionalKey(Schema.Unknown)", "payload: jsonRequest(Schema.Unknown)"} {
		if !strings.Contains(api, want) {
			t.Fatalf("missing %q in compatibility schema", want)
		}
	}
	if strings.Contains(api, "Hidden") {
		t.Fatal("custom decoder implementation fields appeared in generated payload")
	}
	received := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value any
		if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		switch r.URL.Path {
		case "/root-decoder":
			if value != "root token" {
				http.Error(w, "custom root token changed", 400)
				return
			}
		case "/quoting", "/exact":
			object, ok := value.(map[string]any)
			if !ok || object["count"] != "0012" || object["enabled"] != "true" || object["name"] != `"quoted name"` || object["ratio"] != "1.25" {
				http.Error(w, "quoted scalars changed", 400)
				return
			}
		case "/nested-decoder":
			object, ok := value.(map[string]any)
			if !ok || object["token"] != "nested token" {
				http.Error(w, "custom nested token changed", 400)
				return
			}
		}
		received <- r.URL.Path
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
		"api.gen.ts":    api,
		"tsconfig.json": `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","ESNext.Disposable"],"module":"NodeNext","moduleResolution":"NodeNext","strict":true,"types":[]},"include":["*.ts"]}`,
		"check.ts": `import { Effect } from "effect"
import { FetchHttpClient } from "effect/http"
import { HttpApiClient } from "effect/http-api"
import { Api } from "./api.gen.js"
await Effect.runPromise(Effect.gen(function*() {
 const client = yield* HttpApiClient.make(Api,{baseUrl:` + "`" + server.URL + "`" + `})
 yield* client.compatibility.recursive({payload:{value:"root",next:{value:"next",next:null},children:[{value:"child",next:{value:"leaf"}}],extra:true}})
 yield* client.compatibility.mutual({payload:{right:{left:{right:null}}}})
 yield* client.compatibility.rootDecoder({payload:"root token"})
 yield* client.compatibility.nestedDecoder({payload:{token:"nested token",tokens:["a","b"]}})
 const quoted = {count:"0012",enabled:"true",name:'"quoted name"',ratio:"1.25",plain:[1,2]}
 yield* client.compatibility.quoting({payload:quoted})
 yield* client.compatibility.exact({payload:quoted})
 if (false) {
  // @ts-expect-error json,string requires a string on the wire
  yield* client.compatibility.quoting({payload:{count:12}})
 }
 yield* Effect.flip(client.compatibility.quoting({payload:{count:12} as never}))
}).pipe(Effect.provide(FetchHttpClient.layer)))
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
			t.Fatalf("compatibility JSON client: %v\n%s", err, out)
		}
	}
	if len(received) != 6 {
		t.Fatalf("received %d valid requests, want 6", len(received))
	}
}
