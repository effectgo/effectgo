package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/effectgo/effectgo/rt/schema"
)

func TestLegacyJSONBodyKeepsURLValidation(t *testing.T) {
	type input struct {
		ID    int8   `path:"id" json:"id"`
		Limit *int   `query:"limit" json:"limit" schema:"min=1"`
		Name  string `json:"name" schema:"nonempty"`
		Count int    `json:"count"`
	}
	for _, tc := range []struct {
		name, body, query, wantName, issue string
		empty, bestEffort, exact           bool
		maxBody                            int64
	}{
		{name: "unknown omitted null", body: `{"name":null,"unknown":true,"ID":7,"LIMIT":8}`, query: "2"},
		{name: "wrong-type body URL fields ignored", body: `{"ID":"ignored","LIMIT":{"invalid":true}}`, query: "2"},
		{name: "exact wrong-type body URL fields ignored", body: `{"ID":"ignored","LIMIT":{"invalid":true}}`, query: "2", exact: true},
		{name: "first value only", body: `{"name":"first"}` + strings.Repeat("x", 8192), query: "2", wantName: "first", maxBody: 32},
		{name: "URL schema", body: `{}`, query: "0", issue: schema.CodeMin},
		{name: "empty rejected", query: "2", issue: schema.CodeSyntax},
		{name: "empty permitted", query: "2", empty: true},
		{name: "whitespace permitted", body: " \n\t", query: "2", empty: true},
		{name: "exact JSON", body: `{"name":"exact"}`, query: "2", wantName: "exact", exact: true},
		{name: "exact rejects second value", body: `{} {}`, query: "2", exact: true, issue: schema.CodeSyntax},
		{name: "exact rejects empty", query: "2", exact: true, empty: true, issue: schema.CodeSyntax},
		{name: "exact whole body limit", body: `{"name":"first"}` + strings.Repeat(" ", 100), query: "2", exact: true, maxBody: 32, issue: "limit"},
		{name: "best effort partial", body: `{"name":"kept","count":"invalid"}`, query: "2", bestEffort: true, wantName: "kept"},
		{name: "best effort body limit", body: `{"name":"` + strings.Repeat("x", 100) + `"}`, query: "2", bestEffort: true, maxBody: 16, issue: "limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := newInputPlan[input](http.MethodDelete, "/{id}", true)
			if err != nil {
				t.Fatal(err)
			}
			plan.jsonBodyEmpty, plan.jsonBodyBestEffort = tc.empty, tc.bestEffort
			plan.jsonBodyExact = tc.exact
			req := httptest.NewRequest(http.MethodDelete, "/42?limit="+tc.query, strings.NewReader(tc.body))
			req.SetPathValue("id", "42")
			maxBody := tc.maxBody
			if maxBody == 0 {
				maxBody = DefaultMaxBody
			}
			got, err := plan.decode(httptest.NewRecorder(), req, maxBody)
			if tc.issue == "limit" {
				if _, ok := errors.AsType[BodyTooLarge](err); !ok {
					t.Fatalf("limit error=%v", err)
				}
			} else if tc.issue != "" {
				de, ok := errors.AsType[*schema.DecodeError](err)
				if !ok || len(de.Issues) == 0 || de.Issues[0].Code != tc.issue {
					t.Fatalf("decode error=%v, want %s", err, tc.issue)
				}
			} else if err != nil || got.ID != 42 || got.Limit == nil || *got.Limit != 2 || got.Name != tc.wantName {
				t.Fatalf("input=%+v err=%v", got, err)
			}
		})
	}
}

type legacyCustomJSONInput struct {
	ID       int `path:"id" json:"id"`
	Value    any `json:"value"`
	decoded  bool
	original string
}

func (v *legacyCustomJSONInput) UnmarshalJSON(data []byte) error {
	type plain legacyCustomJSONInput
	v.original = string(data)
	if err := json.Unmarshal(data, (*plain)(v)); err != nil {
		return err
	}
	v.decoded = true
	return nil
}

func TestLegacyJSONBodyHonorsCustomDecoder(t *testing.T) {
	const body = `{"id":7, "value": { "nested":true }}`
	for _, exact := range []bool{false, true} {
		plan, err := newInputPlan[legacyCustomJSONInput](http.MethodPost, "/{id}", true)
		if err != nil {
			t.Fatal(err)
		}
		plan.jsonBodyExact = exact
		req := httptest.NewRequest(http.MethodPost, "/42", strings.NewReader(body))
		req.SetPathValue("id", "42")
		got, err := plan.decode(httptest.NewRecorder(), req, DefaultMaxBody)
		if err != nil || got.ID != 42 || !got.decoded || got.Value == nil || got.original != body {
			t.Fatalf("custom JSON input=%+v err=%v", got, err)
		}
	}
}

type legacyURLMode string

func (v legacyURLMode) Valid() bool { return v == "safe" }

func TestLegacyJSONBodyValidatesAndNormalizesURLIndependently(t *testing.T) {
	type input struct {
		Name string        `query:"name" json:"name" schema:"trim,nonempty"`
		Mode legacyURLMode `query:"mode" json:"mode"`
		Data []byte        `json:"data"`
	}
	plan, err := newInputPlan[input](http.MethodPost, "/", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"safe", "unsafe"} {
		req := httptest.NewRequest(http.MethodPost, "/?name=%20alice%20&mode="+mode, strings.NewReader(`{"data":"eA=="}`))
		got, err := plan.decode(httptest.NewRecorder(), req, DefaultMaxBody)
		if mode == "unsafe" {
			de, ok := errors.AsType[*schema.DecodeError](err)
			if !ok || len(de.Issues) != 1 || de.Issues[0].Code != schema.CodeEnum || strings.Join(de.Issues[0].Path, ".") != "mode" {
				t.Fatalf("invalid URL enum accepted: input=%+v err=%v", got, err)
			}
		} else if err != nil || got.Name != "alice" || got.Mode != "safe" || string(got.Data) != "x" {
			t.Fatalf("validated URL normalization lost: input=%+v err=%v", got, err)
		}
	}
}

type legacyEOFJSONInput struct {
	Value string `json:"value"`
}

func (*legacyEOFJSONInput) UnmarshalJSON([]byte) error { return io.EOF }

func TestLegacyJSONBodyEmptyDoesNotSuppressCustomEOF(t *testing.T) {
	for _, exact := range []bool{false, true} {
		plan, err := newInputPlan[legacyEOFJSONInput](http.MethodPost, "/", true)
		if err != nil {
			t.Fatal(err)
		}
		plan.jsonBodyEmpty, plan.jsonBodyExact = true, exact
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
		_, err = plan.decode(httptest.NewRecorder(), req, DefaultMaxBody)
		de, ok := errors.AsType[*schema.DecodeError](err)
		if !ok || len(de.Issues) != 1 || de.Issues[0].Message != "EOF" {
			t.Fatalf("custom EOF suppressed (exact=%v): %v", exact, err)
		}
	}
}

type legacyNestedJSONValue struct {
	seen []string
}

func (v *legacyNestedJSONValue) UnmarshalJSON(data []byte) error {
	v.seen = append(v.seen, string(data))
	return nil
}

func TestLegacyJSONBodyFilteringPreservesNestedDecoding(t *testing.T) {
	type input struct {
		ID     int                   `path:"id" json:"id"`
		Nested legacyNestedJSONValue `json:"nested"`
	}
	for _, exact := range []bool{false, true} {
		plan, err := newInputPlan[input](http.MethodPost, "/{id}", true)
		if err != nil {
			t.Fatal(err)
		}
		plan.jsonBodyExact = exact
		req := httptest.NewRequest(http.MethodPost, "/42", strings.NewReader(`{"id":"ignored","nested":{ "first": 1 },"nested":{ "second": 2 }}`))
		req.SetPathValue("id", "42")
		got, err := plan.decode(httptest.NewRecorder(), req, DefaultMaxBody)
		if err != nil || got.ID != 42 || strings.Join(got.Nested.seen, "|") != `{ "first": 1 }|{ "second": 2 }` {
			t.Fatalf("nested decoder bytes or duplicate keys changed: input=%+v err=%v", got, err)
		}
	}
}
