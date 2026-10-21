package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/effectgo/effectgo/rt/web"
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

type migrationOperation struct {
	OperationID string `json:"operationId"`
	Responses   map[string]struct {
		Content map[string]struct {
			Schema struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Type string `json:"type"`
				} `json:"properties"`
			} `json:"schema"`
		} `json:"content"`
	} `json:"responses"`
}

// The example checks its explicit flat JSON envelopes and application-owned
// problem encoder. It does not interpret arbitrary OpenAPI.
func checkMigrationContract(operation migrationOperation, operationID string, status int, media string, body []byte) error {
	if operation.OperationID != operationID {
		return fmt.Errorf("operation ID %q, want %q", operation.OperationID, operationID)
	}
	response, ok := operation.Responses[strconv.Itoa(status)]
	if !ok {
		return fmt.Errorf("status %d is not documented", status)
	}
	entry, ok := response.Content[media]
	if !ok {
		return fmt.Errorf("media %q is not documented for status %d", media, status)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		return fmt.Errorf("body is not a JSON object: %w", err)
	}
	for _, key := range entry.Schema.Required {
		if _, ok := value[key]; !ok {
			return fmt.Errorf("body lacks documented field %q", key)
		}
	}
	for key, property := range entry.Schema.Properties {
		v, present := value[key]
		if !present {
			continue
		}
		switch property.Type {
		case "string":
			if _, ok := v.(string); !ok {
				return fmt.Errorf("body field %q is not a string", key)
			}
		case "integer":
			if n, ok := v.(float64); !ok || n != float64(int64(n)) {
				return fmt.Errorf("body field %q is not an integer", key)
			}
		default:
			return fmt.Errorf("body field %q has unsupported documented type %q", key, property.Type)
		}
	}
	return nil
}

func TestMigrationHTTPContract(t *testing.T) {
	var doc struct {
		Paths map[string]struct {
			Get migrationOperation `json:"get"`
		} `json:"paths"`
	}
	if err := json.Unmarshal([]byte(migrationOpenAPI), &doc); err != nil {
		t.Fatal(err)
	}
	operation := doc.Paths["/api/auth/me"].Get
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
			if err := checkMigrationContract(operation, "get-api-auth-me", res.StatusCode, res.Header.Get("Content-Type"), body); err != nil {
				t.Fatal(err)
			}
			// Each negative changes the contract or the captured response from this
			// same HTTP request; no encoder behavior is inferred from its Go type.
			for _, bad := range []struct {
				name, id, media string
				status          int
				body            []byte
			}{
				{"operation ID", "changed-operation", res.Header.Get("Content-Type"), res.StatusCode, body},
				{"status", "get-api-auth-me", res.Header.Get("Content-Type"), 418, body},
				{"media", "get-api-auth-me", "text/plain", res.StatusCode, body},
				{"body", "get-api-auth-me", res.Header.Get("Content-Type"), res.StatusCode, []byte(`{}`)},
				{"body type", "get-api-auth-me", res.Header.Get("Content-Type"), res.StatusCode, []byte(`{"data":42,"detail":42,"status":"500","title":42}`)},
			} {
				if err := checkMigrationContract(operation, bad.id, bad.status, bad.media, bad.body); err == nil || !strings.Contains(err.Error(), strings.Fields(bad.name)[0]) {
					t.Errorf("%s mismatch did not fail explicitly: %v", bad.name, err)
				}
			}
		})
	}
}
