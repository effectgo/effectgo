package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postBody(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/x/1", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func maxBodyRouter(opts ...Option) *Router {
	r := New(opts...)
	Post(r, "/x/{id}", func(ctx context.Context, in UpdateIn) (NoContent, error) {
		return NoContent{}, nil
	})
	return r
}

func TestDefaultMaxBodyWithoutMiddleware(t *testing.T) {
	r := maxBodyRouter()
	big := strings.Repeat("x", int(DefaultMaxBody)+1)
	rec := postBody(r, big)
	if rec.Code != http.StatusRequestEntityTooLarge || rec.Body.String() != `{"_tag":"BodyTooLarge"}` {
		t.Errorf("1 MiB+1 without middleware: %d %s", rec.Code, rec.Body)
	}
	exact := ` {"name":"ok"}` + strings.Repeat(" ", int(DefaultMaxBody)-len(` {"name":"ok"}`))
	if rec := postBody(r, exact); rec.Code != http.StatusNoContent {
		t.Errorf("exactly 1 MiB rejected: %d %s", rec.Code, rec.Body)
	}
}

func TestWithMaxBodyLowered(t *testing.T) {
	r := maxBodyRouter(WithMaxBody(16))
	if rec := postBody(r, strings.Repeat("x", 17)); rec.Code != 413 {
		t.Errorf("17 bytes over a 16 limit: %d", rec.Code)
	}
	if rec := postBody(r, `{"name":"x"}`); rec.Code != http.StatusNoContent {
		t.Errorf("12 bytes under a 16 limit: %d %s", rec.Code, rec.Body)
	}
}

func TestWithMaxBodyRaised(t *testing.T) {
	r := maxBodyRouter(WithMaxBody(8 << 20))
	body := strings.Repeat(" ", int(DefaultMaxBody)+1) + `{"name":"x"}`
	if rec := postBody(r, body); rec.Code != http.StatusNoContent {
		t.Errorf("over 1 MiB under an 8 MiB limit: %d %s", rec.Code, rec.Body)
	}
}

func TestWithMaxBodyPanicsOnNonPositive(t *testing.T) {
	mustPanicWith(t, "must be positive", func() { WithMaxBody(0) })
	mustPanicWith(t, "must be positive", func() { WithMaxBody(-5) })
}
