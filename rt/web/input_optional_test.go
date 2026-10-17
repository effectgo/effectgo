package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOptionalHeaderAndCookiePresence(t *testing.T) {
	type input struct {
		Header *string `header:"X-Optional" json:"header"`
		Cookie *string `cookie:"optional" json:"cookie"`
	}
	for _, tc := range []struct {
		name, header, cookie                  string
		cookiePresent, wantHeader, wantCookie bool
	}{
		{name: "absent"},
		{name: "header only", header: "value", wantHeader: true},
		{name: "cookie only", cookie: "value", cookiePresent: true, wantCookie: true},
		{name: "empty cookie is present", cookiePresent: true, wantCookie: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			r := New()
			Get(r, "/optional", func(_ context.Context, in input) (NoContent, error) {
				called = true
				if (in.Header != nil) != tc.wantHeader || (in.Cookie != nil) != tc.wantCookie {
					t.Errorf("presence: header=%v cookie=%v", in.Header, in.Cookie)
				}
				if in.Header != nil && *in.Header != tc.header {
					t.Errorf("header=%q", *in.Header)
				}
				if in.Cookie != nil && *in.Cookie != tc.cookie {
					t.Errorf("cookie=%q", *in.Cookie)
				}
				return NoContent{}, nil
			})
			req := httptest.NewRequest(http.MethodGet, "/optional", nil)
			req.Header.Set("X-Optional", tc.header)
			if tc.cookiePresent {
				req.AddCookie(&http.Cookie{Name: "optional", Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if !called || w.Code != http.StatusNoContent {
				t.Fatalf("handler called=%v, status=%d: %s", called, w.Code, w.Body)
			}
		})
	}
}

func TestRequiredHeaderAndCookieRejectMissing(t *testing.T) {
	type input struct {
		Header string `header:"X-Required" json:"header"`
		Cookie string `cookie:"required" json:"cookie"`
	}
	for _, missing := range []string{"header", "cookie"} {
		t.Run(missing, func(t *testing.T) {
			called := false
			r := New()
			Get(r, "/required", func(context.Context, input) (NoContent, error) {
				called = true
				return NoContent{}, nil
			})
			req := httptest.NewRequest(http.MethodGet, "/required", nil)
			if missing != "header" {
				req.Header.Set("X-Required", "value")
			}
			if missing != "cookie" {
				req.AddCookie(&http.Cookie{Name: "required", Value: "value"})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if called || w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"_tag":"DecodeError"`) {
				t.Fatalf("handler called=%v, status=%d: %s", called, w.Code, w.Body)
			}
		})
	}
}
