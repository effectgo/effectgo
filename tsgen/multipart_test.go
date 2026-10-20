package tsgen

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A generated client must send binary file contents and let fetch set the boundary.
func TestMultipartClientUpload(t *testing.T) {
	node, tsc, nm := requireTypeScript(t)
	api, err := generate(t, "testdata/mod", "./ok/upload")
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json" {
			var body struct {
				Item struct {
					ID int `json:"id"`
				} `json:"item"`
			}
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&body); err != nil || body.Item.ID != 7 {
				http.Error(w, "wrong JSON shape", 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"json","size":7}`))
			return
		}
		if r.Method != "POST" || r.URL.Path != "/upload/7" {
			http.Error(w, "wrong route", 400)
			return
		}
		reader, err := r.MultipartReader()
		if err != nil {
			http.Error(w, "missing multipart boundary", 400)
			return
		}
		part, err := reader.NextPart()
		if err != nil {
			http.Error(w, "missing file", 400)
			return
		}
		data, err := io.ReadAll(part)
		if err != nil || part.FormName() != "file" || part.FileName() != "references.xlsx" || string(data) != "\x00\xffsheet\r\n" || part.Header.Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
			http.Error(w, "corrupted file", 400)
			return
		}
		if _, err := reader.NextPart(); err != io.EOF {
			http.Error(w, "unexpected part", 400)
			return
		}
		received <- string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"references.xlsx","size":9}`))
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
const body = new FormData()
body.append("file", new Blob([new Uint8Array([0,255,115,104,101,101,116,13,10])], {type:"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}), "references.xlsx")
const result = await Effect.runPromise(Effect.gen(function*() {
 const client = yield* HttpApiClient.make(Api, {baseUrl: ` + "`" + server.URL + "`" + `})
 return yield* client.upload.upload({params:{id:7}, payload:body})
}).pipe(Effect.provide(FetchHttpClient.layer)))
if (result.name !== "references.xlsx" || result.size !== 9) throw new Error("wrong decoded response")
const jsonResult = await Effect.runPromise(Effect.gen(function*() {
 const client = yield* HttpApiClient.make(Api, {baseUrl: ` + "`" + server.URL + "`" + `})
 return yield* client.upload.json({payload:{item:{id:7}}})
}).pipe(Effect.provide(FetchHttpClient.layer)))
if (jsonResult.name !== "json" || jsonResult.size !== 7) throw new Error("multipart type polluted JSON schema")
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
			t.Fatalf("generated upload client: %v\n%s", err, out)
		}
	}
	select {
	case <-received:
	default:
		t.Fatal("server did not receive upload")
	}
}

func TestMultipartInvalidContracts(t *testing.T) {
	_, err := generate(t, "testdata/mod", "./bad/upload")
	wantErr(t, err, "multipart file fields cannot be mixed with JSON body fields", "multipart form name file is repeated", "web.File or *web.File", "GET takes no body", "multipart file cannot be embedded or have schema or other input location tags")
}
