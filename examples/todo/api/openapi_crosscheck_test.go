package api

// Runtime cross-check of the OpenAPI error docs. The documents come from
// testdata/openapi/effectgo-merged.json, which regen-openapi.sh produces with
// effectgo openapi -merge. For PUT /v3/todos/{id}, each status observed over
// HTTP must appear as a response of that operation in the document, and that
// response must list the variant that produces the status: ErrConflict for the
// duplicate title, and the unclassified "?" for the malformed form body (both
// 500, so the status alone would not tell them apart).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/effectgo/effectgo/examples/todo/domain"
)

const mergedDoc = "../testdata/openapi/effectgo-merged.json"

// putResponses returns the documented responses of PUT /v3/todos/{id}: for each
// response key, the variants that the response lists in x-effectgo-errors.
func putResponses(t *testing.T) map[string]map[string]bool {
	t.Helper()
	data, err := os.ReadFile(mergedDoc)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Errors []struct {
					Variant string `json:"variant"`
				} `json:"x-effectgo-errors"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	op, ok := doc.Paths["/v3/todos/{id}"]["put"]
	if !ok {
		t.Fatalf("%s has no PUT /v3/todos/{id}", mergedDoc)
	}
	out := map[string]map[string]bool{}
	for k, resp := range op.Responses {
		variants := map[string]bool{}
		for _, e := range resp.Errors {
			variants[e.Variant] = true
		}
		out[k] = variants
	}
	return out
}

func TestPutV3ObservedStatusesAreDocumented(t *testing.T) {
	documented := putResponses(t)

	svc := domain.NewService()
	if _, err := svc.Create("buy milk"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create("bread"); err != nil {
		t.Fatal(err)
	}
	h := NewServer(svc).Mux

	const jsonType = "application/json"
	const formType = "application/x-www-form-urlencoded"
	// via is the variant that the documented response of the observed status
	// must list: the attribution of the status, not only its existence.
	cases := []struct {
		name, path, body, contentType string
		want                          int
		via                           string
	}{
		{"invalid title", "/v3/todos/1", `{"title":"   "}`, jsonType, 400, "domain.ErrInvalid"},
		{"missing id", "/v3/todos/999", `{"done":true}`, jsonType, 404, "domain.ErrNotFound"},
		{"duplicate title (ErrConflict)", "/v3/todos/2", `{"title":"buy milk"}`, jsonType, 500, "domain.ErrConflict"},
		{"malformed form body", "/v3/todos/1", `title=%zz`, formType, 500, "?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, c.path, strings.NewReader(c.body))
			req.Header.Set("Content-Type", c.contentType)
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("observed %d, want %d (body %s)", rec.Code, c.want, rec.Body.String())
			}
			key := strconv.Itoa(rec.Code)
			variants, ok := documented[key]
			if !ok {
				t.Fatalf("observed %d is not a response of PUT /v3/todos/{id} in %s (documented: %v)", rec.Code, mergedDoc, documented)
			}
			if !variants[c.via] {
				t.Errorf("the %s response does not list %q (it lists %v)", key, c.via, variants)
			}
		})
	}
}
