package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/effectgo/effectgo/rt/schema"
)

// ---- fixtures ----

type Contact struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type notFound struct {
	Detail string `json:"detail"`
}

func (e notFound) Error() string { return e.Detail }
func (notFound) StatusCode() int { return http.StatusNotFound }

type teapot struct{ Brew string }

func (*teapot) Error() string   { return "teapot" }
func (*teapot) StatusCode() int { return http.StatusTeapot }

type leaky struct{ secret string }

func (leaky) Error() string { return "db password is hunter2" }

type badStatus struct{}

func (badStatus) Error() string   { return "ok?" }
func (badStatus) StatusCode() int { return 200 }

type unmarshalable struct{}

func (unmarshalable) Error() string   { return "x" }
func (unmarshalable) StatusCode() int { return 409 }
func (unmarshalable) MarshalJSON() ([]byte, error) {
	return nil, errors.New("cannot marshal")
}

type scalarErr string

func (e scalarErr) Error() string { return string(e) }
func (scalarErr) StatusCode() int { return 422 }

type numberErr struct {
	Big int64 `json:"big"`
}

func (numberErr) Error() string   { return "big" }
func (numberErr) StatusCode() int { return 409 }

type Service struct {
	contacts map[int64]Contact
}

type GetIn struct {
	ID    int64     `path:"id" json:"id" schema:"min=1"`
	Page  int       `query:"page" json:"page" schema:"min=1"`
	Q     string    `query:"q" json:"q" schema:"trim,max=5"`
	Limit *int      `query:"limit" json:"limit"`
	Tags  []string  `query:"tag" json:"tags"`
	Flag  bool      `query:"flag" json:"flag"`
	Since time.Time `query:"since" json:"since"`
	Ratio float64   `query:"ratio" json:"ratio"`
}
type HeaderCookieIn struct {
	RequestID string `header:"X-Request-ID" json:"request_id"`
	Session   string `cookie:"session" json:"session"`
	CSRF      string `cookie:"csrf" json:"csrf"`
}

func TestHeaderAndCookieInputs(t *testing.T) {
	r := New()
	Get(r, "/auth/me", func(ctx context.Context, in HeaderCookieIn) (Contact, error) {
		return Contact{ID: 1, Name: in.RequestID + ":" + in.Session + ":" + in.CSRF}, nil
	})

	rec := do(t, r, http.MethodGet, "/auth/me", "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"session"`) {
		t.Fatalf("missing cookie: %d %s", rec.Code, rec.Body)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("X-Request-ID", "req-7")
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "token"})
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"id":1,"name":"req-7:abc:token"}` {
		t.Fatalf("header/cookie input: %d %s", rec.Code, rec.Body)
	}
}

type UpdateIn struct {
	ID   int64   `path:"id" json:"id"`
	Name string  `json:"name" schema:"trim,nonempty"`
	Note *string `json:"note"`
}

func (s *Service) Get(ctx context.Context, in GetIn) (Contact, error) {
	c, ok := s.contacts[in.ID]
	if !ok {
		return Contact{}, notFound{Detail: fmt.Sprintf("contact %d not found", in.ID)}
	}
	return c, nil
}

func (s *Service) Update(ctx context.Context, in UpdateIn) (Contact, error) {
	c := Contact{ID: in.ID, Name: in.Name}
	s.contacts[in.ID] = c
	return c, nil
}

type deleteIn struct {
	ID int64 `path:"id" json:"id"`
}

func (s *Service) Find(ctx context.Context, in deleteIn) (Contact, error) {
	c, ok := s.contacts[in.ID]
	if !ok {
		return Contact{}, notFound{Detail: fmt.Sprintf("contact %d not found", in.ID)}
	}
	return c, nil
}

func (s *Service) Delete(ctx context.Context, in deleteIn) (NoContent, error) {
	delete(s.contacts, in.ID)
	return NoContent{}, nil
}

func newService() *Service {
	return &Service{contacts: map[int64]Contact{1: {ID: 1, Name: "Ada"}}}
}

func quietLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, nil))
}

func do(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", rec.Body.String(), err)
	}
	return m
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("network gone") }

// ---- routing and typed input ----

func TestGetDecodesPathQueryAndEncodesOutput(t *testing.T) {
	r := New()
	svc := newService()
	Get(r, "/api/contacts/{id}", svc.Get)

	rec := do(t, r, "GET", "/api/contacts/1?page=2&q=%20ab%20&limit=5&tag=x&tag=y&flag=true&since=2026-10-15T00:00:00Z&ratio=0.5", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type %q", got)
	}
	if rec.Body.String() != `{"id":1,"name":"Ada"}` {
		t.Errorf("body %q", rec.Body.String())
	}

	rec = do(t, r, "GET", "/api/contacts/999?page=1&q=a&tag=x&flag=false&since=2026-10-15T00:00:00Z&ratio=1", "")
	m := decodeMap(t, rec)
	if rec.Code != 404 || m["_tag"] != "notFound" || m["detail"] != "contact 999 not found" {
		t.Errorf("not found: %d %v", rec.Code, m)
	}
}

func TestPostReadsBodyAndPathAndDecodeErrors(t *testing.T) {
	r := New()
	svc := newService()
	Post(r, "/api/contacts/{id}", svc.Update)

	rec := do(t, r, "POST", "/api/contacts/7", `{"name":"Grace"}`)
	if rec.Code != 200 || rec.Body.String() != `{"id":7,"name":"Grace"}` {
		t.Fatalf("ok case: %d %s", rec.Code, rec.Body)
	}

	rec = do(t, r, "POST", "/api/contacts/7", `{"name":"   "}`)
	m := decodeMap(t, rec)
	if rec.Code != http.StatusBadRequest || m["_tag"] != "DecodeError" {
		t.Errorf("trimmed empty name: %d %v", rec.Code, m)
	}

	rec = do(t, r, "POST", "/api/contacts/abc", `{"name":"x"}`)
	m = decodeMap(t, rec)
	if rec.Code != 400 || m["_tag"] != "DecodeError" {
		t.Fatalf("bad path: %d %v", rec.Code, m)
	}
	first := m["issues"].([]any)[0].(map[string]any)
	if first["code"] != "type" || first["path"].([]any)[0] != "id" {
		t.Errorf("issue %v", first)
	}

	rec = do(t, r, "POST", "/api/contacts/7", `{"name":"x","extra":1}`)
	if rec.Code != 400 || decodeMap(t, rec)["_tag"] != "DecodeError" {
		t.Errorf("unknown body field: %d %s", rec.Code, rec.Body)
	}

	rec = do(t, r, "POST", "/api/contacts/7", `{"name":"x","id":9}`)
	m = decodeMap(t, rec)
	if rec.Code != 400 || m["_tag"] != "DecodeError" {
		t.Fatalf("body field named like a path field: %d %v", rec.Code, m)
	}
	if got := m["issues"].([]any)[0].(map[string]any)["code"]; got != "unknown" {
		t.Errorf("collision code %v", got)
	}

	rec = do(t, r, "POST", "/api/contacts/7", `{"name":`)
	if rec.Code != 400 || decodeMap(t, rec)["_tag"] != "DecodeError" {
		t.Errorf("malformed JSON: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, r, "POST", "/api/contacts/7", `[1]`)
	if rec.Code != 400 || decodeMap(t, rec)["_tag"] != "DecodeError" {
		t.Errorf("array body: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, r, "POST", "/api/contacts/7", `null`)
	if rec.Code != 400 {
		t.Errorf("null body: %d", rec.Code)
	}
	rec = do(t, r, "POST", "/api/contacts/7", `"text"`)
	if rec.Code != 400 {
		t.Errorf("string body: %d", rec.Code)
	}
	// An empty body is an object with no fields: the required field is missing.
	rec = do(t, r, "POST", "/api/contacts/7", "")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"missing"`) {
		t.Errorf("empty body: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, r, "POST", "/api/contacts/7", `{"name":"x","note":"hi"}`)
	if rec.Code != 200 {
		t.Errorf("optional pointer body field: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, r, "POST", "/api/contacts/7", ` {"name":"x"} `)
	if rec.Code != 200 {
		t.Errorf("surrounding whitespace: %d %s", rec.Code, rec.Body)
	}
}

func TestQueryValues(t *testing.T) {
	r := New()
	var got GetIn
	Get(r, "/q/{id}", func(ctx context.Context, in GetIn) (Contact, error) {
		got = in
		return Contact{}, nil
	})
	rec := do(t, r, "GET", "/q/3?page=1&q=ok&tag=a&tag=b&flag=false&ratio=1e2&since=2026-01-02T03:04:05Z", "")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got.ID != 3 || got.Page != 1 || got.Limit != nil || len(got.Tags) != 2 || got.Tags[1] != "b" || got.Flag || got.Ratio != 100 || got.Since.Year() != 2026 {
		t.Errorf("parsed %+v", got)
	}
	rec = do(t, r, "GET", "/q/3?page=x&q=ok&flag=yes&ratio=NaN&since=later&limit=2.5&tag=ok", "")
	m := decodeMap(t, rec)
	if rec.Code != 400 {
		t.Fatalf("bad query: %d %v", rec.Code, m)
	}
	want := map[string]bool{"page": true, "flag": true, "ratio": true, "since": true, "limit": true}
	for _, raw := range m["issues"].([]any) {
		is := raw.(map[string]any)
		delete(want, is["path"].([]any)[0].(string))
		if is["code"] != "type" {
			t.Errorf("issue %v is not a type issue", is)
		}
	}
	if len(want) != 0 {
		t.Errorf("no issue for %v in %s", want, rec.Body)
	}
	rec = do(t, r, "GET", "/q/3?page=4&page=9&q=ok&tag=x&flag=true&ratio=1&since=2026-10-15T00:00:00Z", "")
	if rec.Code != 200 || got.Page != 4 {
		t.Errorf("first value: %d page=%d", rec.Code, got.Page)
	}
	rec = do(t, r, "GET", "/q/3?page=1&q=toolong&tag=x&flag=true&ratio=1&since=2026-10-15T00:00:00Z", "")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"max"`) {
		t.Errorf("schema rule on query: %d %s", rec.Code, rec.Body)
	}
}

func TestDeleteNoContentAndNoInput(t *testing.T) {
	r := New()
	svc := newService()
	Delete(r, "/api/contacts/{id}", svc.Delete)
	rec := do(t, r, "DELETE", "/api/contacts/1", "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("204: %d %q", rec.Code, rec.Body)
	}
	if _, ok := svc.contacts[1]; ok {
		t.Errorf("handler not called")
	}

	r2 := New()
	Get(r2, "/ping", func(ctx context.Context, _ NoInput) (Contact, error) {
		return Contact{ID: 9}, nil
	})
	rec = do(t, r2, "GET", "/ping?anything=1", "")
	if rec.Code != 200 || rec.Body.String() != `{"id":9,"name":""}` {
		t.Errorf("NoInput: %d %s", rec.Code, rec.Body)
	}
	// NoInput never reads the body, even for POST.
	Post(r2, "/ping", func(ctx context.Context, _ NoInput) (NoContent, error) {
		return NoContent{}, nil
	})
	req := httptest.NewRequest("POST", "/ping", failingReader{})
	rec = httptest.NewRecorder()
	r2.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("NoInput POST read the body: %d", rec.Code)
	}
}

func TestBodyReadFailureIsDecodeError(t *testing.T) {
	r := New()
	Post(r, "/x/{id}", func(ctx context.Context, in UpdateIn) (NoContent, error) {
		return NoContent{}, nil
	})
	req := httptest.NewRequest("POST", "/x/1", failingReader{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	m := decodeMap(t, rec)
	if rec.Code != 400 || m["_tag"] != "DecodeError" {
		t.Errorf("read failure: %d %v", rec.Code, m)
	}
}

func TestOutputEncodingFailureIs500(t *testing.T) {
	var buf bytes.Buffer
	type wrapper struct {
		V any `json:"v"`
	}
	r := New(WithLogger(quietLogger(&buf)))
	Get(r, "/bad", func(ctx context.Context, _ NoInput) (wrapper, error) {
		return wrapper{V: func() {}}, nil
	})
	rec := do(t, r, "GET", "/bad", "")
	if rec.Code != 500 || rec.Body.String() != `{"_tag":"InternalError"}` {
		t.Errorf("encode failure: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(buf.String(), "encode response failed") {
		t.Errorf("no log: %s", buf.String())
	}
}

// ---- registration panics ----

func mustPanicWith(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		v := recover()
		if v == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if !strings.Contains(fmt.Sprint(v), want) {
			t.Fatalf("panic %v, want it to contain %q", v, want)
		}
	}()
	f()
}

type chanOut struct {
	C chan int `json:"c"`
}

type inBadPattern struct {
	ID int64 `path:"nope" json:"id"`
}

type inBothTags struct {
	ID int64 `path:"id" query:"id" json:"id"`
}

type inEmptyQuery struct {
	Q string `query:"" json:"q"`
}

type inStructSlice struct {
	Q []struct{ A int } `query:"q" json:"q"`
}

type inDashJSON struct {
	ID int64 `path:"id" json:"-"`
}

type inBodyOnGet struct {
	Body string `json:"body"`
}

type inNonScalarPath struct {
	P struct{ A int } `path:"p" json:"p"`
}

type inBadField struct {
	Sub struct{ X chan int } `json:"sub"`
}

func TestRegistrationPanics(t *testing.T) {
	none := func(context.Context, NoInput) (NoContent, error) { return NoContent{}, nil }
	mustPanicWith(t, "nil handler", func() { Get[GetIn, Contact](New(), "/x/{id}", nil) })
	mustPanicWith(t, "output", func() {
		Get(New(), "/x", func(context.Context, NoInput) (chanOut, error) { return chanOut{}, nil })
	})
	mustPanicWith(t, "must be a struct", func() {
		Get(New(), "/x", func(context.Context, int) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, "input:", func() {
		Get(New(), "/x", func(context.Context, inBadField) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, "is not in pattern", func() {
		Get(New(), "/x", func(context.Context, inBadPattern) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, "both path and query", func() {
		Get(New(), "/x/{id}", func(context.Context, inBothTags) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, "empty name", func() {
		Get(New(), "/x", func(context.Context, inEmptyQuery) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, "scalar or a slice", func() {
		Get(New(), "/x", func(context.Context, inStructSlice) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, `json:"-"`, func() {
		Get(New(), "/x/{id}", func(context.Context, inDashJSON) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, "takes no body", func() {
		Get(New(), "/x", func(context.Context, inBodyOnGet) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, "must be a scalar", func() {
		Post(New(), "/x/{p}", func(context.Context, inNonScalarPath) (NoContent, error) { return NoContent{}, nil })
	})
	mustPanicWith(t, "is NoInput", func() {
		Post(New(), "/x/{id}", none)
	})
}

func TestRouterPanicsOnDuplicateRoute(t *testing.T) {
	r := New()
	none := func(context.Context, NoInput) (NoContent, error) { return NoContent{}, nil }
	Get(r, "/dup", none)
	mustPanicWith(t, "conflicts", func() { Get(r, "/dup", none) })
}

// ---- error encoding ----

func TestErrorBodyAndStatuses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		tag    string
		field  string
	}{
		{"value status", notFound{Detail: "gone"}, 404, "notFound", "detail"},
		{"pointer status", &teapot{Brew: "oolong"}, 418, "teapot", "Brew"},
		{"wrapped", fmt.Errorf("layer: %w", notFound{Detail: "x"}), 404, "notFound", "detail"},
		{"no status", leaky{secret: "s"}, 500, "InternalError", ""},
		{"invalid status", badStatus{}, 500, "InternalError", ""},
		{"not an object", scalarErr("boom"), 422, "scalarErr", ""},
		{"marshal fails", unmarshalable{}, 409, "unmarshalable", ""},
		{"InternalError", InternalError{}, 500, "InternalError", ""},
		{"BodyTooLarge", BodyTooLarge{}, 413, "BodyTooLarge", ""},
		{"DecodeError", &schema.DecodeError{Issues: []schema.Issue{{Path: []string{"a"}, Code: "min", Message: "m"}}}, 400, "DecodeError", "issues"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := ErrorBody(tc.err)
			if status != tc.status {
				t.Errorf("status %d, want %d", status, tc.status)
			}
			if body["_tag"] != tc.tag {
				t.Errorf("_tag %v, want %s", body["_tag"], tc.tag)
			}
			if tc.field != "" {
				if _, ok := body[tc.field]; !ok {
					t.Errorf("field %s missing from %v", tc.field, body)
				}
			}
			if tc.status == 500 && len(body) != 1 {
				t.Errorf("500 body must hold only the tag: %v", body)
			}
			if strings.Contains(fmt.Sprint(body), "hunter2") {
				t.Errorf("error text leaked: %v", body)
			}
		})
	}
	if status, body := ErrorBody(errors.New("plain")); status != 500 || body["_tag"] != "InternalError" {
		t.Errorf("plain error: %d %v", status, body)
	}
}

func TestErrorTextNeverReachesTheClient(t *testing.T) {
	var buf bytes.Buffer
	r := New(WithLogger(quietLogger(&buf)))
	Get(r, "/leak", func(context.Context, NoInput) (NoContent, error) {
		return NoContent{}, leaky{secret: "pw"}
	})
	rec := do(t, r, "GET", "/leak", "")
	if rec.Code != 500 || rec.Body.String() != `{"_tag":"InternalError"}` {
		t.Errorf("response %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(buf.String(), "hunter2") {
		t.Errorf("error not logged: %s", buf.String())
	}
}

func TestErrorBodyPreservesLargeIntegers(t *testing.T) {
	_, body := ErrorBody(numberErr{Big: 9007199254740993})
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), "9007199254740993") {
		t.Errorf("integer precision lost: %s", raw)
	}
}

func TestCustomErrorEncoder(t *testing.T) {
	var buf bytes.Buffer
	r := New(
		WithLogger(quietLogger(&buf)),
		WithErrorEncoder(func(w http.ResponseWriter, req *http.Request, err error) {
			status, body := ErrorBody(err)
			if status == 404 {
				body["detail"] = body["detail"].(string) + " (custom)"
			}
			writeJSON(w, status, body)
		}),
	)
	svc := newService()
	Get(r, "/c/{id}", svc.Find)
	rec := do(t, r, "GET", "/c/5", "")
	m := decodeMap(t, rec)
	if rec.Code != 404 || m["detail"] != "contact 5 not found (custom)" {
		t.Errorf("custom encoder: %d %v", rec.Code, m)
	}
	Get(r, "/leak", func(context.Context, NoInput) (NoContent, error) { return NoContent{}, leaky{} })
	if rec := do(t, r, "GET", "/leak", ""); rec.Code != 500 {
		t.Errorf("custom encoder unknown error: %d", rec.Code)
	}
	Post(r, "/decode/{id}", svc.Update)
	if rec := do(t, r, "POST", "/decode/1", `{"name":1}`); rec.Code != 400 {
		t.Errorf("custom encoder decode error: %d", rec.Code)
	}
}

// ---- panics ----

func TestPanicBeforeWriteBecomes500(t *testing.T) {
	var buf bytes.Buffer
	r := New(WithLogger(quietLogger(&buf)))
	Get(r, "/boom", func(context.Context, NoInput) (NoContent, error) {
		panic("kaboom")
	})
	rec := do(t, r, "GET", "/boom", "")
	if rec.Code != 500 || rec.Body.String() != `{"_tag":"InternalError"}` {
		t.Errorf("panic response %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(buf.String(), "handler panic") || !strings.Contains(buf.String(), "kaboom") {
		t.Errorf("panic not logged: %s", buf.String())
	}
}

func TestPanicAfterWriteIsRethrown(t *testing.T) {
	var buf bytes.Buffer
	started := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(200)
			next.ServeHTTP(w, req)
		})
	}
	r := New(WithLogger(quietLogger(&buf)), WithMiddleware(started))
	Get(r, "/half", func(context.Context, NoInput) (NoContent, error) {
		panic("late")
	})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic after the response started was swallowed")
			}
		}()
		do(t, r, "GET", "/half", "")
	}()
	if !strings.Contains(buf.String(), "late") {
		t.Errorf("panic not logged: %s", buf.String())
	}
}

func TestAbortHandlerIsRethrown(t *testing.T) {
	r := New(WithLogger(quietLogger(&bytes.Buffer{})))
	Get(r, "/abort", func(context.Context, NoInput) (NoContent, error) {
		panic(http.ErrAbortHandler)
	})
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Errorf("ErrAbortHandler not rethrown: %v", v)
		}
	}()
	do(t, r, "GET", "/abort", "")
}

func TestUnstatusedErrorIsLoggedWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	r := New(WithLogger(quietLogger(&buf)), WithMiddleware(RequestID()))
	Get(r, "/x", func(context.Context, NoInput) (NoContent, error) {
		return NoContent{}, errors.New("disk on fire")
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Request-ID", "req-42")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 500 || !strings.Contains(buf.String(), "disk on fire") || !strings.Contains(buf.String(), "req-42") {
		t.Errorf("unstatused error: %d log=%s", rec.Code, buf.String())
	}
}

func TestDefaultLoggerAndMiddlewareOrder(t *testing.T) {
	if New().logger == nil {
		t.Fatal("default logger missing")
	}
	var order []string
	mw := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	r := New(WithMiddleware(mw("outer"), mw("inner")))
	Get(r, "/o", func(context.Context, NoInput) (NoContent, error) {
		order = append(order, "handler")
		return NoContent{}, nil
	})
	do(t, r, "GET", "/o", "")
	if strings.Join(order, ",") != "outer,inner,handler" {
		t.Errorf("order %v", order)
	}
}

func TestStatusOfRejectsNonErrorStatuses(t *testing.T) {
	if _, ok := statusOf(badStatus{}); ok {
		t.Error("200 treated as an error status")
	}
	if s, ok := statusOf(notFound{}); !ok || s != 404 {
		t.Errorf("statusOf(notFound) = %d %v", s, ok)
	}
	if typeName(&teapot{}) != "teapot" || typeName(struct{ error }{}) != "Error" {
		t.Errorf("typeName")
	}
}

// ---- panic handler ----

// spyWriter counts the writes that reach the response, so a test can see
// whether the router has answered before the panic handler runs.
type spyWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (s *spyWriter) WriteHeader(code int) {
	s.writes++
	s.ResponseRecorder.WriteHeader(code)
}

func (s *spyWriter) Write(b []byte) (int, error) {
	s.writes++
	return s.ResponseRecorder.Write(b)
}

// TestPanicHandlerSeesPanicBeforeTheResponse: the function gets the value, the
// request ID from the context, and the request, before the 500 is written; the
// response is unchanged.
func TestPanicHandlerSeesPanicBeforeTheResponse(t *testing.T) {
	var buf bytes.Buffer
	spy := &spyWriter{ResponseRecorder: httptest.NewRecorder()}
	var calls, writesAtCall int
	var got any
	var gotID, gotPath string
	r := New(WithLogger(quietLogger(&buf)), WithMiddleware(RequestID()),
		WithPanicHandler(func(ctx context.Context, req *http.Request, recovered any) {
			calls++
			writesAtCall = spy.writes
			got, gotID, gotPath = recovered, RequestIDFrom(ctx), req.URL.Path
		}))
	Get(r, "/boom", func(context.Context, NoInput) (NoContent, error) {
		panic("kaboom")
	})
	req := httptest.NewRequest("GET", "/boom", nil)
	req.Header.Set("X-Request-ID", "req-7")
	r.ServeHTTP(spy, req)
	if calls != 1 || got != "kaboom" || gotID != "req-7" || gotPath != "/boom" {
		t.Errorf("panic handler: calls=%d value=%v request_id=%q path=%q", calls, got, gotID, gotPath)
	}
	if writesAtCall != 0 {
		t.Errorf("panic handler ran after %d writes, want none", writesAtCall)
	}
	if spy.Code != 500 || spy.Body.String() != `{"_tag":"InternalError"}` {
		t.Errorf("response %d %s, want the 500 InternalError body", spy.Code, spy.Body)
	}
}

// TestPanicHandlerPanicIsLogged: a panic in the function is logged, and the
// response is still the router's 500.
func TestPanicHandlerPanicIsLogged(t *testing.T) {
	var buf bytes.Buffer
	r := New(WithLogger(quietLogger(&buf)), WithPanicHandler(func(context.Context, *http.Request, any) {
		panic("second")
	}))
	Get(r, "/boom", func(context.Context, NoInput) (NoContent, error) {
		panic("kaboom")
	})
	rec := do(t, r, "GET", "/boom", "")
	if rec.Code != 500 || rec.Body.String() != `{"_tag":"InternalError"}` {
		t.Errorf("response %d %s, want the 500 InternalError body", rec.Code, rec.Body)
	}
	if !strings.Contains(buf.String(), "panic handler panic") || !strings.Contains(buf.String(), "second") {
		t.Errorf("panic of the handler not logged: %s", buf.String())
	}
}

// TestPanicHandlerSeesLatePanicThenRethrow: after the response has started, the
// function still sees the panic, and the router still re-raises it.
func TestPanicHandlerSeesLatePanicThenRethrow(t *testing.T) {
	started := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(200)
			next.ServeHTTP(w, req)
		})
	}
	var calls int
	r := New(WithLogger(quietLogger(&bytes.Buffer{})), WithMiddleware(started),
		WithPanicHandler(func(context.Context, *http.Request, any) { calls++ }))
	Get(r, "/half", func(context.Context, NoInput) (NoContent, error) {
		panic("late")
	})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic after the response started was swallowed")
			}
		}()
		do(t, r, "GET", "/half", "")
	}()
	if calls != 1 {
		t.Errorf("panic handler called %d times, want 1", calls)
	}
}

// TestPanicHandlerSkipsAbortHandler: http.ErrAbortHandler is net/http's own abort,
// not a failure, so the function does not see it.
func TestPanicHandlerSkipsAbortHandler(t *testing.T) {
	var calls int
	r := New(WithLogger(quietLogger(&bytes.Buffer{})),
		WithPanicHandler(func(context.Context, *http.Request, any) { calls++ }))
	Get(r, "/abort", func(context.Context, NoInput) (NoContent, error) {
		panic(http.ErrAbortHandler)
	})
	func() {
		defer func() {
			if v := recover(); v != http.ErrAbortHandler {
				t.Errorf("ErrAbortHandler not rethrown: %v", v)
			}
		}()
		do(t, r, "GET", "/abort", "")
	}()
	if calls != 0 {
		t.Errorf("panic handler saw ErrAbortHandler %d times", calls)
	}
}

// TestWithPanicHandlerNilPanics: a nil function is a programming error at startup.
func TestWithPanicHandlerNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("WithPanicHandler(nil) did not panic")
		}
	}()
	WithPanicHandler(nil)
}
