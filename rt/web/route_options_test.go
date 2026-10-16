package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type externalUnauthorized struct {
	Detail string `json:"detail"`
}

func (externalUnauthorized) Error() string   { return "unauthorized" }
func (externalUnauthorized) StatusCode() int { return http.StatusUnauthorized }
func externalDenied(context.Context, NoInput) (NoContent, error) {
	return NoContent{}, externalUnauthorized{Detail: "login required"}
}

func TestExternalErrorPreservesCustomEnvelope(t *testing.T) {
	r := New(WithErrorEncoder(func(w http.ResponseWriter, req *http.Request, err error) {
		status, body := ErrorBody(err)
		delete(body, "_tag")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	Get(r, "/private", externalDenied, ExternalError[externalUnauthorized]())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/private", nil))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusUnauthorized || len(body) != 1 || body["detail"] != "login required" {
		t.Fatalf("custom envelope changed: %d %s", w.Code, w.Body.String())
	}
}

func TestExternalErrorRejectsInvalidBoundary(t *testing.T) {
	for name, option := range map[string]RouteOption{
		"status":      ExternalError[badStatus](),
		"pointer":     ExternalError[*teapot](),
		"zero option": {},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid boundary accepted at startup")
				}
			}()
			Get(New(), "/private", externalDenied, option)
		})
	}
}
