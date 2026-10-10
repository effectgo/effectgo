package api

import (
	"context"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestMergedDocumentValidates loads the merged OpenAPI document with kin-openapi
// and validates it against the OpenAPI schema.
func TestMergedDocumentValidates(t *testing.T) {
	data, err := os.ReadFile(mergedDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate: %v", err)
	}
}
