package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faintghost/effectgo/rt/web"
)

// These are alternatives, so absence must reach the handler rather than fail
// decoding. A non-pointer field would instead require that particular input.
type migrationInput struct {
	Actor  *string `header:"X-Actor" json:"actor"`
	Local  *string `cookie:"session" json:"local"`
	Secure *string `cookie:"__Host-session" json:"secure"`
}

type migrationOutput struct {
	Data string `json:"data"`
}

type migrationUnauthorized struct {
	Detail string `json:"detail"`
}

func (migrationUnauthorized) Error() string   { return "unauthorized" }
func (migrationUnauthorized) StatusCode() int { return http.StatusUnauthorized }

func migrationRead(_ context.Context, in migrationInput) (migrationOutput, error) {
	for _, value := range []*string{in.Actor, in.Secure, in.Local} {
		if value != nil && *value != "" {
			return migrationOutput{Data: *value}, nil
		}
	}
	return migrationOutput{}, migrationUnauthorized{Detail: "login required"}
}

func migrationFailure(context.Context, web.NoInput) (migrationOutput, error) {
	return migrationOutput{}, errors.New("private failure")
}

// Custom encoding owns both the wire body and its Content-Type. ErrorBody
// redacts internal errors before this encoder adds a problem response envelope.
func migrationEncodeError(w http.ResponseWriter, _ *http.Request, err error) {
	status, body := web.ErrorBody(err)
	delete(body, "_tag")
	mediaType := "application/json"
	if status == http.StatusInternalServerError {
		mediaType = "application/problem+json"
		body = map[string]any{"status": status, "title": "Internal Server Error"}
	}
	w.Header().Set("Content-Type", mediaType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// A hand-maintained OpenAPI adapter must retain the old operationId, describe
// the actual top-level envelope and match each encoder response's media type.
// This document is independent of the runtime: route options do not supply it.
const migrationOpenAPI = `{
  "openapi": "3.1.0",
  "info": {"title": "Migration example", "version": "example"},
  "paths": {"/api/auth/me": {"get": {
    "operationId": "get-api-auth-me",
    "responses": {
      "200": {"description": "Authenticated", "content": {"application/json": {"schema": {"type": "object", "required": ["data"], "properties": {"data": {"type": "string"}}}}}},
      "401": {"description": "Login required", "content": {"application/json": {"schema": {"type": "object", "required": ["detail"], "properties": {"detail": {"type": "string"}}}}}},
      "500": {"description": "Internal error", "content": {"application/problem+json": {"schema": {"type": "object", "required": ["status", "title"], "properties": {"status": {"type": "integer"}, "title": {"type": "string"}}}}}}
    }
  }}}
}`

func TestMigrationHTTPContract(t *testing.T) {
	var doc struct {
		Paths map[string]struct {
			Get struct {
				OperationID string `json:"operationId"`
				Responses   map[string]struct {
					Content map[string]struct {
						Schema struct {
							Required []string `json:"required"`
						} `json:"schema"`
					} `json:"content"`
				} `json:"responses"`
			} `json:"get"`
		} `json:"paths"`
	}
	if err := json.Unmarshal([]byte(migrationOpenAPI), &doc); err != nil {
		t.Fatal(err)
	}
	operation := doc.Paths["/api/auth/me"].Get
	if operation.OperationID != "get-api-auth-me" {
		t.Fatalf("operation ID changed: %q", operation.OperationID)
	}
	for _, tc := range []struct {
		name, actor, cookie, status, want string
		fail                              bool
	}{
		{name: "local cookie", cookie: "session=local", status: "200", want: `{"data":"local"}`},
		{name: "secure cookie", cookie: "__Host-session=secure", status: "200", want: `{"data":"secure"}`},
		{name: "secure takes precedence", cookie: "session=local; __Host-session=secure", status: "200", want: `{"data":"secure"}`},
		{name: "actor without cookies", actor: "actor", status: "200", want: `{"data":"actor"}`},
		{name: "missing credentials reaches authentication", status: "401", want: `{"detail":"login required"}`},
		{name: "internal error", status: "500", want: `{"status":500,"title":"Internal Server Error"}`, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := web.New(web.WithLogger(quietLogger()), web.WithErrorEncoder(migrationEncodeError))
			if tc.fail {
				web.Get(r, "/api/auth/me", migrationFailure)
			} else {
				web.Get(r, "/api/auth/me", migrationRead, web.ExternalError[migrationUnauthorized]())
			}
			server := httptest.NewServer(r)
			defer server.Close()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/auth/me", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.actor != "" {
				req.Header.Set("X-Actor", tc.actor)
			}
			if tc.cookie != "" {
				req.Header.Set("Cookie", tc.cookie)
			}
			res, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status[:3] != tc.status {
				t.Fatalf("status %s: %s", res.Status, body)
			}
			var got, want map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("unexpected envelope: %s", body)
			}
			for key, value := range want {
				if got[key] != value {
					t.Fatalf("body %s, want %s", body, tc.want)
				}
			}
			mediaType := res.Header.Get("Content-Type")
			content := operation.Responses[tc.status].Content
			entry, ok := content[mediaType]
			if !ok {
				t.Fatalf("undocumented Content-Type %q for %s", mediaType, tc.status)
			}
			for _, key := range entry.Schema.Required {
				if _, ok := got[key]; !ok {
					t.Fatalf("missing documented field %q: %s", key, body)
				}
			}
		})
	}
}
