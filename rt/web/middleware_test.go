package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func serve(h http.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequestID(t *testing.T) {
	h := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFrom(r.Context()) == "" {
			t.Error("no request ID in context")
		}
		okHandler(w, r)
	}))
	rec := serve(h, "GET", "/", "", map[string]string{"X-Request-ID": "abc-123.x_y"})
	if rec.Header().Get("X-Request-ID") != "abc-123.x_y" {
		t.Errorf("accepted ID not echoed: %q", rec.Header().Get("X-Request-ID"))
	}
	for _, bad := range []string{"", "has space", "bad/slash", strings.Repeat("a", 129)} {
		rec = serve(h, "GET", "/", "", map[string]string{"X-Request-ID": bad})
		id := rec.Header().Get("X-Request-ID")
		if len(id) != 32 || !validRequestID(id) {
			t.Errorf("header %q: generated id %q", bad, id)
		}
	}
	if RequestIDFrom(context.Background()) != "" {
		t.Error("empty context has an ID")
	}
}

func TestAccessLogFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := New(WithMiddleware(RequestID(), AccessLog(logger)))
	Get(r, "/api/contacts/{id}", func(ctx context.Context, in struct {
		ID int64 `path:"id" json:"id"`
	}) (struct{}, error) {
		return struct{}{}, nil
	})
	rec := serve(r, "GET", "/api/contacts/5?secret=q", "", map[string]string{"X-Request-ID": "req-1"})
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	line := strings.TrimSpace(buf.String())
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("log line %q: %v", line, err)
	}
	if m["request_id"] != "req-1" || m["method"] != "GET" || m["route"] != "GET /api/contacts/{id}" || m["status"] != float64(200) {
		t.Errorf("fields %v", m)
	}
	if _, ok := m["duration_ms"]; !ok {
		t.Errorf("no duration_ms: %v", m)
	}
	if strings.Contains(line, "secret") {
		t.Errorf("query value logged: %s", line)
	}

	buf.Reset()
	rec = serve(r, "GET", "/nope", "", nil)
	if rec.Code != 404 || !strings.Contains(buf.String(), `"route":""`) || !strings.Contains(buf.String(), `"status":404`) {
		t.Errorf("unmatched route log: %d %s", rec.Code, buf.String())
	}

	buf.Reset()
	req := httptest.NewRequest("BREW", "/x", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	if !strings.Contains(buf.String(), `"method":"OTHER"`) {
		t.Errorf("unknown method not normalised: %s", buf.String())
	}

	buf.Reset()
	fail := New(WithMiddleware(AccessLog(logger)))
	Get(fail, "/boom", func(context.Context, struct{}) (struct{}, error) {
		return struct{}{}, notFound{Detail: "x"}
	})
	serve(fail, "GET", "/boom", "", nil)
	if !strings.Contains(buf.String(), `"level":"INFO"`) {
		t.Errorf("4xx should log at INFO: %s", buf.String())
	}

	fail2 := New(WithMiddleware(AccessLog(nil)))
	Get(fail2, "/err", func(context.Context, struct{}) (struct{}, error) {
		return struct{}{}, leaky{}
	})
	serve(fail2, "GET", "/err", "", nil)
}

func TestAccessLogLevelForServerErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := New(WithMiddleware(AccessLog(logger)), WithLogger(logger))
	Get(r, "/err", func(context.Context, struct{}) (struct{}, error) {
		return struct{}{}, leaky{}
	})
	serve(r, "GET", "/err", "", nil)
	if !strings.Contains(buf.String(), `"level":"ERROR"`) {
		t.Errorf("5xx should log at ERROR: %s", buf.String())
	}
}

func TestHostCheck(t *testing.T) {
	h := HostCheck("127.0.0.1:8080", "localhost:8080")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("allowed host: %d", rec.Code)
	}
	req = httptest.NewRequest("GET", "/", nil)
	req.Host = "evil.example"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 || rec.Body.String() != `{"_tag":"HostNotAllowed"}` {
		t.Errorf("forbidden host: %d %s", rec.Code, rec.Body)
	}
	mustPanicWith(t, "allowed host", func() { HostCheck() })
}

func TestTokenGuard(t *testing.T) {
	h := TokenGuard("x-token", "s3cret")(http.HandlerFunc(okHandler))
	cases := []struct {
		method, token string
		status        int
	}{
		{"GET", "", 200},
		{"HEAD", "", 200},
		{"POST", "", 403},
		{"POST", "wrong", 403},
		{"POST", "s3cre", 403},
		{"POST", "s3cret-extra", 403},
		{"PUT", "s3cret", 200},
		{"DELETE", "s3cret", 200},
	}
	for _, tc := range cases {
		rec := serve(h, tc.method, "/", "", map[string]string{"x-token": tc.token})
		if rec.Code != tc.status {
			t.Errorf("%s token %q: %d, want %d", tc.method, tc.token, rec.Code, tc.status)
		}
	}
	rec := serve(h, "POST", "/", "", nil)
	if rec.Body.String() != `{"_tag":"TokenMismatch"}` {
		t.Errorf("body %s", rec.Body)
	}
	mustPanicWith(t, "non-empty token", func() { TokenGuard("x-token", "") })
	mustPanicWith(t, "non-empty token", func() { TokenGuard("", "x") })
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders("default-src 'self'")(http.HandlerFunc(okHandler))
	rec := serve(h, "GET", "/", "", nil)
	if rec.Header().Get("Cache-Control") != "no-store" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		rec.Header().Get("Content-Security-Policy") != "default-src 'self'" {
		t.Errorf("headers %v", rec.Header())
	}
	rec = serve(SecurityHeaders("")(http.HandlerFunc(okHandler)), "GET", "/", "", nil)
	if rec.Header().Get("Content-Security-Policy") != "" {
		t.Errorf("empty CSP should not be set")
	}
}

func TestBodyLimitContentLength(t *testing.T) {
	var seen string
	h := BodyLimit(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = string(b)
		okHandler(w, r)
	}))
	rec := serve(h, "POST", "/", "0123456789", map[string]string{"Content-Length": "10"})
	if rec.Code != 200 || seen != "0123456789" {
		t.Errorf("limit exactly: %d %q", rec.Code, seen)
	}
	seen = ""
	rec = serve(h, "POST", "/", "01234567890", map[string]string{"Content-Length": "11"})
	if rec.Code != 413 || rec.Body.String() != `{"_tag":"BodyTooLarge"}` || seen != "" {
		t.Errorf("declared over limit: %d %s handler=%q", rec.Code, rec.Body, seen)
	}
	rec = serve(h, "POST", "/", "x", map[string]string{"Content-Length": "abc"})
	if rec.Code != 500 || rec.Body.String() != `{"_tag":"InternalError"}` {
		t.Errorf("bad Content-Length: %d %s", rec.Code, rec.Body)
	}
	mustPanicWith(t, "non-negative", func() { BodyLimit(-1) })
}

func TestBodyLimitOnRouterStreamingOverflow(t *testing.T) {
	r := New(WithMiddleware(BodyLimit(8)))
	Post(r, "/x/{id}", func(ctx context.Context, in UpdateIn) (NoContent, error) {
		return NoContent{}, nil
	})
	req := httptest.NewRequest("POST", "/x/1", strings.NewReader(`{"name":"a very long name"}`))
	req.ContentLength = -1
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 413 || rec.Body.String() != `{"_tag":"BodyTooLarge"}` {
		t.Errorf("streamed overflow: %d %s", rec.Code, rec.Body)
	}
}

func TestMiddlewaresComposeInRouter(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := New(
		WithLogger(logger),
		WithMiddleware(
			RequestID(),
			AccessLog(logger),
			SecurityHeaders("default-src 'none'"),
			HostCheck("example.test"),
			TokenGuard("x-token", "tok"),
			BodyLimit(100),
		),
	)
	Post(r, "/api/contact/{id}", func(ctx context.Context, in UpdateIn) (Contact, error) {
		return Contact{ID: in.ID, Name: in.Name}, nil
	})
	Get(r, "/api/contact/{id}", newService().Get)

	req := httptest.NewRequest("POST", "/api/contact/3", strings.NewReader(`{"name":"Ada"}`))
	req.Host = "example.test"
	req.Header.Set("x-token", "tok")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != `{"id":3,"name":"Ada"}` {
		t.Errorf("composed ok: %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Request-ID") == "" {
		t.Errorf("headers %v", rec.Header())
	}
	if !strings.Contains(buf.String(), `"status":200`) {
		t.Errorf("access log missing: %s", buf.String())
	}

	req = httptest.NewRequest("POST", "/api/contact/3", strings.NewReader(`{}`))
	req.Host = "example.test"
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("token guard in router: %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/contact/1", nil)
	req.Host = "other.test"
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("host check in router: %d", rec.Code)
	}
}

func TestStatic(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "index.html"), "<!doctype html><title>app</title>")
	mustWrite(t, filepath.Join(dir, "assets", "app.js"), "console.log(1)")
	mustWrite(t, filepath.Join(dir, "secret.txt"), "top secret")
	h := Static(dir)

	rec := serve(h, "GET", "/", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>app</title>") {
		t.Errorf("index: %d %s", rec.Code, rec.Body)
	}
	rec = serve(h, "GET", "/contacts/42", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "app</title>") {
		t.Errorf("SPA fallback: %d", rec.Code)
	}
	rec = serve(h, "GET", "/assets/app.js", "", nil)
	if rec.Code != 200 || rec.Body.String() != "console.log(1)" {
		t.Errorf("asset: %d %q", rec.Code, rec.Body)
	}
	rec = serve(h, "HEAD", "/assets/app.js", "", nil)
	if rec.Code != 200 {
		t.Errorf("HEAD asset: %d", rec.Code)
	}
	rec = serve(h, "GET", "/assets/missing.js", "", nil)
	if rec.Code != 404 || rec.Body.String() != `{"_tag":"NotFound"}` {
		t.Errorf("missing asset: %d %s", rec.Code, rec.Body)
	}
	rec = serve(h, "GET", "/assets/", "", nil)
	if rec.Code != 404 {
		t.Errorf("assets dir: %d", rec.Code)
	}
	rec = serve(h, "GET", "/assets/../secret.txt", "", nil)
	if rec.Code == 200 {
		t.Errorf("traversal served: %d %s", rec.Code, rec.Body)
	}
	rec = serve(h, "GET", "/api/contacts", "", nil)
	if rec.Code != 404 || rec.Body.String() != `{"_tag":"NotFound"}` {
		t.Errorf("api prefix: %d %s", rec.Code, rec.Body)
	}
	rec = serve(h, "POST", "/", "", nil)
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST: %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}

	custom := Static(dir, WithAPIPrefix("/rpc"))
	rec = serve(custom, "GET", "/rpc/x", "", nil)
	if rec.Code != 404 {
		t.Errorf("custom API prefix: %d", rec.Code)
	}
	rec = serve(custom, "GET", "/api/x", "", nil)
	if rec.Code != 200 {
		t.Errorf("api prefix not replaced: %d", rec.Code)
	}
	noAPI := Static(dir, WithAPIPrefix(""))
	if rec := serve(noAPI, "GET", "/api/x", "", nil); rec.Code != 200 {
		t.Errorf("empty API prefix should serve everything: %d", rec.Code)
	}

	empty := Static(t.TempDir())
	rec = serve(empty, "GET", "/", "", nil)
	if rec.Code != 500 || rec.Body.String() != `{"_tag":"InternalError"}` {
		t.Errorf("missing index: %d %s", rec.Code, rec.Body)
	}
	rec = serve(empty, "GET", "/assets/x.js", "", nil)
	if rec.Code != 404 {
		t.Errorf("missing asset with no build: %d", rec.Code)
	}
	rec = serve(h, "GET", "/assets/sub", "", nil)
	if rec.Code != 404 {
		t.Errorf("directory as asset: %d", rec.Code)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
