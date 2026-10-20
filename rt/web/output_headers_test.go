package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOutputHeadersPreserveSessionCookie(t *testing.T) {
	type output struct {
		Cookie    http.Cookie `header:"Set-Cookie" json:"-"`
		Links     []string    `header:"Link" json:"-"`
		RequestID *string     `header:"X-Request-ID" json:"-"`
		Missing   *string     `header:"X-Missing" json:"-"`
		OK        bool        `json:"ok"`
	}
	r := New()
	Get(r, "/login", func(context.Context, NoInput) (*output, error) {
		return &output{Cookie: http.Cookie{Name: "__Host-session", Value: "opaque", Secure: true, HttpOnly: true, Path: "/", SameSite: http.SameSiteStrictMode}, Links: []string{"</first>", "</second>"}, RequestID: new("request-7"), OK: true}, nil
	})
	server := httptest.NewServer(r)
	defer server.Close()
	response, err := http.Get(server.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	cookies := response.Cookies()
	if response.StatusCode != 200 || len(cookies) != 1 || cookies[0].Name != "__Host-session" || cookies[0].Value != "opaque" || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie changed: %d %+v", response.StatusCode, cookies)
	}
	if values := response.Header.Values("Link"); len(values) != 2 || values[0] != "</first>" || values[1] != "</second>" {
		t.Fatalf("Link values changed: %v", values)
	}
	if response.Header.Get("X-Request-ID") != "request-7" || response.Header.Get("X-Missing") != "" {
		t.Fatal("optional headers changed")
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || body["ok"] != true {
		t.Fatalf("headers leaked to JSON: %v", body)
	}
}
