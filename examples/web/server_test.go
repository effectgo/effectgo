package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return m
}

func TestGetContact(t *testing.T) {
	h := NewHandler(NewStore(), quietLogger())

	rec := do(t, h, "GET", "/api/contacts/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	m := decode(t, rec)
	if m["id"] != float64(1) || m["name"] != "Ada" || m["email"] != "ada@example.com" {
		t.Errorf("body = %v", m)
	}

	rec = do(t, h, "GET", "/api/contacts/999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	m = decode(t, rec)
	if m["_tag"] != "ContactNotFound" {
		t.Errorf("_tag = %v", m["_tag"])
	}
	if d, ok := m["detail"].(string); !ok || d == "" {
		t.Errorf("detail = %v", m["detail"])
	}
	if len(m) != 2 {
		t.Errorf("unexpected fields in %v", m)
	}

	rec = do(t, h, "GET", "/api/contacts/abc", "")
	if rec.Code != http.StatusBadRequest || decode(t, rec)["_tag"] != "DecodeError" {
		t.Errorf("non-integer id: status %d body %s", rec.Code, rec.Body)
	}
}

func TestUpdateContact(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		status  int
		tag     string
		wantOut map[string]any
	}{
		{
			name:    "ok",
			body:    `{"id":1,"name":"Ada Lovelace","email":"ada@example.org"}`,
			status:  http.StatusOK,
			wantOut: map[string]any{"id": float64(1), "name": "Ada Lovelace", "email": "ada@example.org"},
		},
		{name: "empty name", body: `{"id":1,"name":"","email":"a@b.c"}`, status: 400, tag: "ValidationError"},
		{name: "blank name", body: `{"id":1,"name":"   ","email":"a@b.c"}`, status: 400, tag: "ValidationError"},
		{name: "newline in name", body: `{"id":1,"name":"Ada\nLovelace","email":"a@b.c"}`, status: 400, tag: "ValidationError"},
		{name: "unknown id", body: `{"id":42,"name":"Bob","email":"b@b.c"}`, status: 404, tag: "ContactNotFound"},
		{name: "malformed JSON", body: `{"id":`, status: 400, tag: "DecodeError"},
		{name: "unknown field", body: `{"id":1,"name":"Ada","email":"a@b.c","x":1}`, status: 400, tag: "DecodeError"},
		{name: "wrong type", body: `{"id":"1","name":"Ada","email":"a@b.c"}`, status: 400, tag: "DecodeError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewStore(), quietLogger())
			rec := do(t, h, "POST", "/api/contact", tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.status, rec.Body)
			}
			m := decode(t, rec)
			if tt.tag == "" {
				for k, v := range tt.wantOut {
					if m[k] != v {
						t.Errorf("%s = %v, want %v", k, m[k], v)
					}
				}
				return
			}
			if m["_tag"] != tt.tag {
				t.Errorf("_tag = %v, want %s", m["_tag"], tt.tag)
			}
			if tt.tag == "DecodeError" {
				if issues, ok := m["issues"].([]any); !ok || len(issues) == 0 {
					t.Errorf("issues = %v", m["issues"])
				}
			} else if d, ok := m["detail"].(string); !ok || d == "" {
				t.Errorf("detail = %v", m["detail"])
			}
		})
	}
}

func TestUpdateIsVisibleToGet(t *testing.T) {
	h := NewHandler(NewStore(), quietLogger())
	if rec := do(t, h, "POST", "/api/contact", `{"id":1,"name":"Grace","email":"g@example.com"}`); rec.Code != 200 {
		t.Fatalf("update status %d", rec.Code)
	}
	rec := do(t, h, "GET", "/api/contacts/1", "")
	if m := decode(t, rec); m["name"] != "Grace" {
		t.Errorf("after update, name = %v", m["name"])
	}
}
