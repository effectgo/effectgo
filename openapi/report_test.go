package openapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// sample is a PUT with a 404 (documented), a 400 and a 500 (not documented), and
// an unknown status.
func sample() []Route {
	return []Route{{
		Method: "PUT", Path: "/items/{id}", Handler: "update(...)", Complete: false,
		Errors: []RouteError{
			{Status: 404, Variant: "domain.ErrNotFound", Via: []string{"Service.Update", "Store.get"}},
			{Status: 400, Variant: "fuego.BadRequestError", Via: []string{"parseID"}},
			{Status: 500, Variant: "domain.ErrConflict"},
			{Status: 0, Variant: "?", Via: []string{"Context.Body"}},
		},
	}, {
		Method: "GET", Path: "/lit", Undocumented: "no errmap", Pos: "x.go:1:1",
	}}
}

func TestFragmentDescriptions(t *testing.T) {
	ops, undoc := Operations(sample())
	if len(undoc) != 1 {
		t.Fatalf("want one undocumented route, got %d", len(undoc))
	}
	frag := Fragment(ops)
	put := frag["paths"].(map[string]any)["/items/{id}"].(map[string]any)["put"].(map[string]any)
	resp := put["responses"].(map[string]any)
	if got := resp["404"].(map[string]any)["description"]; got != "Not Found: domain.ErrNotFound (via Service.Update → Store.get)" {
		t.Errorf("404 description %q", got)
	}
	if got := resp["default"].(map[string]any)["description"]; got != "Unknown status: ? (via Context.Body)" {
		t.Errorf("default description %q", got)
	}
	ext := resp["500"].(map[string]any)[ExtErrors].([]any)[0].(map[string]any)
	if ext["variant"] != "domain.ErrConflict" || ext["via"] != "" {
		t.Errorf("x-effectgo-errors %v", ext)
	}
	if _, ok := frag["paths"].(map[string]any)["/lit"]; ok {
		t.Errorf("an undocumented route must not appear in the fragment")
	}
}

func TestMergeKeepsOtherKeysAndReplacesErrors(t *testing.T) {
	doc := map[string]any{
		"openapi":    "3.1.0",
		"components": map[string]any{"schemas": map[string]any{"HTTPError": map[string]any{"type": "object"}}},
		"paths": map[string]any{
			"/items/{id}": map[string]any{
				"put": map[string]any{
					"summary": "keep me",
					"responses": map[string]any{
						"200": map[string]any{"description": "ok"},
						"404": map[string]any{"description": "old"},
						"500": map[string]any{"description": "generic"},
					},
				},
			},
		},
	}
	ops, _ := Operations(sample())
	merged, notes := Merge(doc, ops)
	if len(notes) != 0 {
		t.Errorf("unexpected notes %v", notes)
	}
	raw, _ := json.Marshal(merged)
	var m struct {
		Openapi string
		Paths   map[string]map[string]struct {
			Summary   string
			Responses map[string]struct {
				Description string
				Content     map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					}
				}
			}
		}
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	put := m.Paths["/items/{id}"]["put"]
	if put.Summary != "keep me" || m.Openapi != "3.1.0" {
		t.Errorf("unrelated keys changed: summary %q openapi %q", put.Summary, m.Openapi)
	}
	if put.Responses["200"].Description != "ok" {
		t.Errorf("a success response changed")
	}
	if put.Responses["500"].Description == "generic" || !strings.Contains(put.Responses["500"].Description, "Internal Server Error") {
		t.Errorf("the 500 was not replaced: %q", put.Responses["500"].Description)
	}
	if ref := put.Responses["404"].Content["application/json"].Schema.Ref; ref != "#/components/schemas/HTTPError" {
		t.Errorf("inserted response does not reference HTTPError: %q", ref)
	}
	if _, ok := put.Responses["default"]; !ok {
		t.Errorf("default was not inserted")
	}
}

func TestMergeNotesMissingOperation(t *testing.T) {
	ops, _ := Operations(sample())
	_, notes := Merge(map[string]any{"paths": map[string]any{}}, ops)
	if len(notes) != 1 || !strings.Contains(notes[0], "PUT /items/{id} is not in the document") {
		t.Errorf("notes %v", notes)
	}
}

func TestCheckClassifiesDrift(t *testing.T) {
	doc := map[string]any{"paths": map[string]any{
		"/items/{id}": map[string]any{"put": map[string]any{"responses": map[string]any{
			"200": map[string]any{}, "404": map[string]any{}, "409": map[string]any{},
		}}},
	}}
	ops, undoc := Operations(sample())
	drifts := Check(doc, ops, undoc)
	var missing, stale, unchecked []string
	for _, d := range drifts {
		switch d.Kind {
		case "missing":
			missing = append(missing, d.Code)
			if !d.Blocks {
				t.Errorf("a missing code must block")
			}
		case "stale":
			stale = append(stale, d.Code)
		case "unchecked":
			unchecked = append(unchecked, d.Op)
		}
	}
	// The sample is not complete (it has a default), so 409 is not stale: the
	// documented 409 is a code that the code does not produce, but it is only
	// checked for complete operations.
	if strings.Join(missing, ",") != "400,500,default" {
		t.Errorf("missing %v", missing)
	}
	if len(stale) != 0 {
		t.Errorf("an incomplete operation must not report stale codes, got %v", stale)
	}
	if len(unchecked) != 1 {
		t.Errorf("unchecked %v", unchecked)
	}
}

func TestCheckStaleOnlyWhenComplete(t *testing.T) {
	doc := map[string]any{"paths": map[string]any{
		"/x": map[string]any{"get": map[string]any{"responses": map[string]any{"500": map[string]any{}}}},
	}}
	ops := []*Op{{Method: "get", Path: "/x", Complete: true, Errors: []RouteError{{Status: 404, Variant: "a"}}}}
	drifts := Check(doc, ops, nil)
	var kinds []string
	for _, d := range drifts {
		kinds = append(kinds, d.Kind+" "+d.Code)
	}
	if strings.Join(kinds, ",") != "missing 404,stale 500" {
		t.Errorf("drifts %v", kinds)
	}
}
