package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faintghost/effectgo/rt/schema"
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

func TestDecodeIssuesKeepDeclaredSourceAndRejectedValue(t *testing.T) {
	type input struct {
		Batch  int `json:"outbound_batch_id" schema:"min=1"`
		Path   int `path:"id" json:"path" schema:"min=1"`
		Query  int `query:"count" json:"query" schema:"min=1"`
		Header int `header:"X-Count" json:"header" schema:"min=1"`
		Cookie int `cookie:"count" json:"cookie" schema:"min=1"`
	}
	plan, err := newInputPlan[input](http.MethodPost, "/pallets/{id}", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, collision := range []string{"", "&outbound_batch_id=9"} {
		req := httptest.NewRequest(http.MethodPost, "/pallets/0?count=00"+collision, strings.NewReader(`{"outbound_batch_id":0}`))
		req.SetPathValue("id", "00")
		req.Header.Set("X-Count", "00")
		req.AddCookie(&http.Cookie{Name: "count", Value: "00"})
		_, err := plan.decode(httptest.NewRecorder(), req, 1024)
		var de *schema.DecodeError
		if !errors.As(err, &de) || len(de.Issues) != 5 {
			t.Fatalf("issues: %v", err)
		}
		for _, issue := range de.Issues {
			wantSource := issue.Path[0]
			if wantSource == "outbound_batch_id" {
				wantSource = "body"
			}
			raw, ok := issue.Value.(json.RawMessage)
			if !ok || string(raw) != "0" {
				t.Fatalf("%s value = %#v", wantSource, issue.Value)
			}
			if issue.Source != wantSource || issue.Code != schema.CodeMin {
				t.Fatalf("issue = %#v", issue)
			}
			wire, err := json.Marshal(issue)
			want := `{"path":["` + issue.Path[0] + `"],"code":"min","message":"` + issue.Message + `"}`
			if err != nil || string(wire) != want {
				t.Fatalf("default issue JSON = %s, error = %v; want %s", wire, err, want)
			}
		}
	}
}

func TestURLIssueValuesKeepJSONTypes(t *testing.T) {
	type input struct {
		Number int    `query:"number" json:"number" schema:"min=1"`
		Text   string `query:"text" json:"text" schema:"min=3"`
	}
	plan, err := newInputPlan[input](http.MethodGet, "/values", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		number, wantNumber, wantCode string
	}{
		{"00", "0", schema.CodeMin},
		{"invalid", `"invalid"`, schema.CodeType},
	} {
		req := httptest.NewRequest(http.MethodGet, "/values?number="+tc.number+"&text=00", nil)
		_, err := plan.decode(httptest.NewRecorder(), req, 1024)
		var de *schema.DecodeError
		if !errors.As(err, &de) || len(de.Issues) != 2 {
			t.Fatalf("issues = %v", err)
		}
		for _, issue := range de.Issues {
			want, code := `"00"`, schema.CodeMin
			if issue.Path[0] == "number" {
				want, code = tc.wantNumber, tc.wantCode
			}
			value, err := json.Marshal(issue.Value)
			if err != nil || string(value) != want || issue.Code != code || issue.Source != "query" {
				t.Fatalf("issue = %#v; JSON value = %s, error = %v; want %s", issue, value, err, want)
			}
		}
	}
}
