package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/faintghost/effectgo/rt/schema"
)

func TestMultipartMixedScalarsAndFile(t *testing.T) {
	type input struct {
		CommitToken string   `form:"commit_token" json:"commit_token" schema:"nonempty"`
		Copies      int8     `form:"copies" json:"copies"`
		Confirmed   bool     `form:"confirmed" json:"confirmed"`
		Note        *string  `form:"note" json:"note"`
		Tags        []string `form:"tags" json:"tags"`
		Document    File     `form:"document" json:"-"`
	}
	for _, optional := range []bool{false, true} {
		t.Run(map[bool]string{false: "optional absent", true: "optional present"}[optional], func(t *testing.T) {
			parts := []multipartTestPart{
				{name: "commit_token", data: "commit-123", scalar: true},
				{name: "copies", data: "2", scalar: true},
				{name: "confirmed", data: "false", scalar: true},
				{name: "tags", data: "return", scalar: true},
				{name: "tags", data: "label", scalar: true},
				{name: "document", filename: "label.pdf", contentType: "application/pdf", data: "%PDF-label"},
			}
			if optional {
				parts = append(parts, multipartTestPart{name: "note", scalar: true})
			}
			body, contentType := multipartTestBody(t, parts...)
			plan, err := newInputPlan[input](http.MethodPost, "/upload", false)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
			req.Header.Set("Content-Type", contentType)
			got, err := plan.decode(httptest.NewRecorder(), req, int64(len(body)))
			if err != nil {
				t.Fatal(err)
			}
			if got.CommitToken != "commit-123" || got.Copies != 2 || got.Confirmed || strings.Join(got.Tags, ",") != "return,label" {
				t.Fatalf("scalar values: %+v", got)
			}
			if (got.Note != nil) != optional || (got.Note != nil && *got.Note != "") {
				t.Fatalf("optional scalar: %+v", got.Note)
			}
			if got.Document.Filename != "label.pdf" || got.Document.ContentType != "application/pdf" || string(got.Document.Data) != "%PDF-label" {
				t.Fatalf("document: %+v", got.Document)
			}
		})
	}
}

func TestMultipartMixedFormRejectsInvalidInput(t *testing.T) {
	type input struct {
		CommitToken string `form:"commit_token" json:"commit_token" schema:"nonempty"`
		Copies      int8   `form:"copies" json:"copies"`
		Confirmed   *bool  `form:"confirmed" json:"confirmed"`
		Document    File   `form:"document" json:"-"`
	}
	file := multipartTestPart{name: "document", filename: "label.pdf", data: "%PDF-label"}
	token := multipartTestPart{name: "commit_token", data: "commit-123", scalar: true}
	copies := multipartTestPart{name: "copies", data: "2", scalar: true}
	for _, tc := range []struct {
		name, code, path string
		parts            []multipartTestPart
	}{
		{name: "missing token", code: schema.CodeMissing, path: "commit_token", parts: []multipartTestPart{file, copies}},
		{name: "empty token", code: schema.CodeNonEmpty, path: "commit_token", parts: []multipartTestPart{file, copies, {name: "commit_token", scalar: true}}},
		{name: "invalid number", code: schema.CodeType, path: "copies", parts: []multipartTestPart{file, token, {name: "copies", scalar: true, data: "two"}}},
		{name: "overflow number", code: schema.CodeType, path: "copies", parts: []multipartTestPart{file, token, {name: "copies", scalar: true, data: "128"}}},
		{name: "invalid bool", code: schema.CodeType, path: "confirmed", parts: []multipartTestPart{file, token, copies, {name: "confirmed", scalar: true, data: "yes"}}},
		{name: "duplicate scalar", code: schema.CodeType, path: "commit_token", parts: []multipartTestPart{file, token, token, copies}},
		{name: "file for scalar", code: schema.CodeType, path: "commit_token", parts: []multipartTestPart{file, copies, {name: "commit_token", filename: "token.txt", data: "commit-123"}}},
		{name: "empty filename for scalar", code: schema.CodeType, path: "commit_token", parts: []multipartTestPart{file, copies, {name: "commit_token", data: "commit-123"}}},
		{name: "unknown scalar", code: schema.CodeUnknown, path: "extra", parts: []multipartTestPart{file, token, copies, {name: "extra", scalar: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			router := New()
			Post(router, "/upload", func(context.Context, input) (NoContent, error) {
				calls.Add(1)
				return NoContent{}, nil
			})
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			body, contentType := multipartTestBody(t, tc.parts...)
			status, data := multipartTestRequest(t, server, "/upload", body, contentType, false)
			var response struct {
				Issues []schema.Issue `json:"issues"`
			}
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			if status != http.StatusBadRequest || calls.Load() != 0 || len(response.Issues) == 0 {
				t.Fatalf("status=%d calls=%d body=%s", status, calls.Load(), data)
			}
			if issue := response.Issues[0]; issue.Code != tc.code || strings.Join(issue.Path, ".") != tc.path {
				t.Fatalf("issue=%+v, want code=%s path=%s", issue, tc.code, tc.path)
			}
		})
	}
}

func TestMultipartMixedFormTotalBodyLimit(t *testing.T) {
	type input struct {
		CommitToken string `form:"commit_token" json:"commit_token"`
		Document    File   `form:"document" json:"-"`
	}
	body, contentType := multipartTestBody(t,
		multipartTestPart{name: "commit_token", data: strings.Repeat("token", 100), scalar: true},
		multipartTestPart{name: "document", filename: "label.pdf", data: "%PDF-label"},
	)
	for _, streamed := range []bool{false, true} {
		for _, over := range []bool{false, true} {
			t.Run(map[bool]string{false: "declared", true: "streamed"}[streamed]+map[bool]string{false: " exact", true: " trailing over"}[over], func(t *testing.T) {
				var calls atomic.Int32
				router := New(WithMaxBody(int64(len(body))))
				Post(router, "/upload", func(context.Context, input) (NoContent, error) {
					calls.Add(1)
					return NoContent{}, nil
				})
				server := httptest.NewServer(router)
				t.Cleanup(server.Close)
				payload := body
				want, wantCalls := http.StatusNoContent, int32(1)
				if over {
					payload = append(bytes.Clone(body), 'x')
					want, wantCalls = http.StatusRequestEntityTooLarge, 0
				}
				status, data := multipartTestRequest(t, server, "/upload", payload, contentType, streamed)
				if status != want || calls.Load() != wantCalls {
					t.Fatalf("status=%d calls=%d body=%s", status, calls.Load(), data)
				}
			})
		}
	}
}

func TestMultipartSpoolsAndCleansUploads(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	type input struct {
		Document File `form:"document" json:"-"`
	}
	plan, err := newInputPlan[input](http.MethodPost, "/upload", false)
	if err != nil {
		t.Fatal(err)
	}
	part := multipartTestPart{name: "document", filename: "large.pdf", data: strings.Repeat("p", int(multipartMemoryLimit)+1)}
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rejected duplicate"}[duplicate], func(t *testing.T) {
			parts := []multipartTestPart{part}
			if duplicate {
				parts = append(parts, multipartTestPart{name: "document", filename: "second.pdf"})
			}
			body, contentType := multipartTestBody(t, parts...)
			req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
			req.Header.Set("Content-Type", contentType)
			got, cleanup, err := plan.decodeWithCleanup(httptest.NewRecorder(), req, int64(len(body)))
			t.Cleanup(cleanup)
			if duplicate {
				if _, ok := errors.AsType[*schema.DecodeError](err); !ok {
					t.Fatalf("duplicate upload error: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if got.Document.Size != int64(len(part.data)) || got.Document.Data != nil || got.Document.path == "" {
					t.Fatalf("upload did not spool: size=%d memory=%d path=%q", got.Document.Size, len(got.Document.Data), got.Document.path)
				}
				for range 2 {
					reader, err := got.Document.Open()
					if err != nil {
						t.Fatal(err)
					}
					n, readErr := io.Copy(io.Discard, reader)
					closeErr := reader.Close()
					if readErr != nil || closeErr != nil || n != got.Document.Size {
						t.Fatalf("reopened upload: bytes=%d read=%v close=%v", n, readErr, closeErr)
					}
				}
				cleanup()
				if _, err := got.Document.Open(); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("temporary upload survived cleanup: %v", err)
				}
			}
			entries, err := os.ReadDir(os.Getenv("TMPDIR"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary uploads leaked: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestMultipartLegacyExtractionRemainsBounded(t *testing.T) {
	type input struct {
		Token string `form:"token" json:"token"`
		File  File   `form:"file" json:"-"`
	}
	plan, err := newInputPlan[input](http.MethodPost, "/upload", false)
	if err != nil {
		t.Fatal(err)
	}
	plan.formBody = true
	body, contentType := multipartTestBody(t,
		multipartTestPart{name: "extra", filename: "ignored", data: strings.Repeat("x", 4096)},
		multipartTestPart{name: "file", scalar: true, data: "wrong kind ignored"},
		multipartTestPart{name: "token", filename: "wrong-kind", data: "ignored"},
		multipartTestPart{name: "file", filename: "first", data: "first file"},
		multipartTestPart{name: "file", filename: "second", data: "second file"},
		multipartTestPart{name: "token", scalar: true, data: "first token"},
		multipartTestPart{name: "token", scalar: true, data: "second token"},
	)
	for _, over := range []bool{false, true} {
		limit := int64(len(body))
		if over {
			limit--
		}
		req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
		req.ContentLength = -1
		req.Header.Set("Content-Type", contentType)
		got, cleanup, err := plan.decodeWithCleanup(httptest.NewRecorder(), req, limit)
		cleanup()
		if over {
			if _, ok := errors.AsType[BodyTooLarge](err); !ok {
				t.Fatalf("legacy extraction bypassed total limit: %v", err)
			}
		} else if err != nil || got.Token != "first token" || got.File.Filename != "first" || string(got.File.Data) != "first file" {
			t.Fatalf("legacy extraction: token=%q file=%+v err=%v", got.Token, got.File, err)
		}
	}
}
