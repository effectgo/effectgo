package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/effectgo/effectgo/rt/schema"
)

type multipartTestPart struct {
	name, filename, contentType, data string
	scalar                            bool
}

func multipartTestBody(t *testing.T, parts ...multipartTestPart) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		params := map[string]string{"name": part.name}
		if !part.scalar {
			params["filename"] = part.filename
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", params))
		if part.contentType != "" {
			header.Set("Content-Type", part.contentType)
		}
		dst, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(dst, part.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func multipartTestRequest(t *testing.T, server *httptest.Server, path string, body []byte, contentType string, streamed bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Upload", "header")
	req.AddCookie(&http.Cookie{Name: "upload", Value: "cookie"})
	if streamed {
		req.ContentLength = -1
	}
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, data
}

func TestMultipartFilesAndURLInputs(t *testing.T) {
	type input struct {
		ID       int    `path:"id" json:"id"`
		Query    string `query:"q" json:"q"`
		Header   string `header:"X-Upload" json:"header"`
		Cookie   string `cookie:"upload" json:"cookie"`
		Document File   `form:"document" json:"-"`
		Preview  *File  `form:"preview" json:"-"`
	}
	for _, optional := range []bool{false, true} {
		t.Run(map[bool]string{false: "optional absent", true: "optional present"}[optional], func(t *testing.T) {
			seen := make(chan input, 1)
			router := New()
			Post(router, "/upload/{id}", func(_ context.Context, in input) (NoContent, error) {
				seen <- in
				return NoContent{}, nil
			})
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			parts := []multipartTestPart{{name: "document", filename: "report.txt", contentType: "text/plain; charset=utf-8", data: "hello\x00world"}}
			if optional {
				parts = append(parts, multipartTestPart{name: "preview", filename: "empty.png", contentType: "image/png"})
			}
			body, contentType := multipartTestBody(t, parts...)
			status, data := multipartTestRequest(t, server, "/upload/42?q=query", body, contentType, false)
			if status != http.StatusNoContent {
				t.Fatalf("status=%d body=%s", status, data)
			}
			select {
			case in := <-seen:
				if in.ID != 42 || in.Query != "query" || in.Header != "header" || in.Cookie != "cookie" {
					t.Errorf("URL inputs: %+v", in)
				}
				if in.Document.Filename != "report.txt" || in.Document.ContentType != "text/plain; charset=utf-8" || string(in.Document.Data) != "hello\x00world" {
					t.Errorf("document: %+v", in.Document)
				}
				if (in.Preview != nil) != optional {
					t.Fatalf("optional presence: %+v", in.Preview)
				}
				if optional && (in.Preview.Filename != "empty.png" || in.Preview.ContentType != "image/png" || len(in.Preview.Data) != 0) {
					t.Errorf("preview: %+v", in.Preview)
				}
			default:
				t.Fatal("handler did not run")
			}
		})
	}
}

func TestMultipartRejectsInvalidRequestsBeforeHandler(t *testing.T) {
	type input struct {
		Document File `form:"document" json:"-"`
	}
	file := multipartTestPart{name: "document", filename: "file.txt", data: "contents"}
	valid, contentType := multipartTestBody(t, file)
	missing, missingType := multipartTestBody(t)
	duplicate, duplicateType := multipartTestBody(t, file, file)
	unknown, unknownType := multipartTestBody(t, file, multipartTestPart{name: "extra", filename: "extra.txt"})
	scalar, scalarType := multipartTestBody(t, multipartTestPart{name: "document", data: "scalar", scalar: true})
	emptyFilename, emptyFilenameType := multipartTestBody(t, multipartTestPart{name: "document"})
	unnamed, unnamedType := multipartTestBody(t, multipartTestPart{filename: "file.txt"})
	for _, tc := range []struct {
		name, contentType, code, path string
		body                          []byte
	}{
		{name: "missing required", body: missing, contentType: missingType, code: schema.CodeMissing, path: "document"},
		{name: "duplicate", body: duplicate, contentType: duplicateType, code: schema.CodeType, path: "document"},
		{name: "unknown", body: unknown, contentType: unknownType, code: schema.CodeUnknown, path: "extra"},
		{name: "scalar part", body: scalar, contentType: scalarType, code: schema.CodeType, path: "document"},
		{name: "empty filename", body: emptyFilename, contentType: emptyFilenameType, code: schema.CodeType, path: "document"},
		{name: "unnamed part", body: unnamed, contentType: unnamedType, code: schema.CodeType},
		{name: "missing boundary", body: valid, contentType: "multipart/form-data", code: schema.CodeSyntax},
		{name: "wrong boundary", body: valid, contentType: "multipart/form-data; boundary=wrong", code: schema.CodeSyntax},
		{name: "JSON body", body: []byte(`{"document":"file"}`), contentType: "application/json", code: schema.CodeSyntax},
		{name: "truncated body", body: valid[:len(valid)-12], contentType: contentType, code: schema.CodeSyntax, path: "document"},
		{name: "malformed headers", body: []byte("--bad\r\nbroken header\r\n\r\ndata\r\n--bad--\r\n"), contentType: "multipart/form-data; boundary=bad", code: schema.CodeSyntax},
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
			status, data := multipartTestRequest(t, server, "/upload", tc.body, tc.contentType, false)
			var response struct {
				Tag    string         `json:"_tag"`
				Issues []schema.Issue `json:"issues"`
			}
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 0 || status != http.StatusBadRequest || response.Tag != "DecodeError" || len(response.Issues) == 0 {
				t.Fatalf("calls=%d status=%d body=%s", calls.Load(), status, data)
			}
			issue := response.Issues[0]
			if issue.Code != tc.code || strings.Join(issue.Path, ".") != tc.path {
				t.Errorf("issue=%+v, want code=%s path=%s", issue, tc.code, tc.path)
			}
		})
	}
}

func TestMultipartTotalBodyLimit(t *testing.T) {
	type input struct {
		Document File `form:"document" json:"-"`
	}
	body, contentType := multipartTestBody(t, multipartTestPart{name: "document", filename: "file", data: "file data"})
	for _, streamed := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			limit  int64
			body   []byte
			status int
		}{
			{name: "exact limit", limit: int64(len(body)), body: body, status: http.StatusNoContent},
			{name: "one byte over", limit: int64(len(body) - 1), body: body, status: http.StatusRequestEntityTooLarge},
			{name: "trailing bytes over", limit: int64(len(body)), body: append(bytes.Clone(body), 'x'), status: http.StatusRequestEntityTooLarge},
			{name: "trailing bytes within", limit: int64(len(body) + 1), body: append(bytes.Clone(body), 'x'), status: http.StatusNoContent},
			{name: "malformed over limit", limit: 4, body: []byte("malformed"), status: http.StatusRequestEntityTooLarge},
		} {
			t.Run(tc.name+map[bool]string{false: " declared", true: " streamed"}[streamed], func(t *testing.T) {
				var calls atomic.Int32
				router := New(WithMaxBody(tc.limit))
				Post(router, "/upload", func(context.Context, input) (NoContent, error) {
					calls.Add(1)
					return NoContent{}, nil
				})
				server := httptest.NewServer(router)
				t.Cleanup(server.Close)
				status, data := multipartTestRequest(t, server, "/upload", tc.body, contentType, streamed)
				if status != tc.status {
					t.Fatalf("status=%d body=%s", status, data)
				}
				if tc.status == http.StatusRequestEntityTooLarge {
					if calls.Load() != 0 || string(data) != `{"_tag":"BodyTooLarge"}` {
						t.Errorf("calls=%d body=%s", calls.Load(), data)
					}
				} else if calls.Load() != 1 {
					t.Errorf("calls=%d", calls.Load())
				}
			})
		}
	}
}

func registerMultipartInput[In any]() {
	Post(New(), "/upload", func(context.Context, In) (NoContent, error) { return NoContent{}, nil })
}

func TestMultipartRegistrationRejectsInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		register   func()
	}{
		{name: "empty name", want: "empty name", register: registerMultipartInput[struct {
			File File `form:"" json:"-"`
		}]},
		{name: "duplicate name", want: "duplicate form", register: registerMultipartInput[struct {
			First  File  `form:"file" json:"-"`
			Second *File `form:"file" json:"-"`
		}]},
		{name: "JSON body", want: "cannot mix", register: registerMultipartInput[struct {
			File File   `form:"file" json:"-"`
			Name string `json:"name"`
		}]},
		{name: "missing JSON exclusion", want: `json:"-"`, register: registerMultipartInput[struct {
			File File `form:"file"`
		}]},
		{name: "JSON name", want: `json:"-"`, register: registerMultipartInput[struct {
			File File `form:"file" json:"file"`
		}]},
		{name: "scalar JSON exclusion", want: "matching JSON name", register: registerMultipartInput[struct {
			File string `form:"file" json:"-"`
		}]},
		{name: "slice", want: "web.File or *web.File", register: registerMultipartInput[struct {
			Files []File `form:"file" json:"-"`
		}]},
		{name: "pointer depth", want: "web.File or *web.File", register: registerMultipartInput[struct {
			File **File `form:"file" json:"-"`
		}]},
		{name: "path conflict", want: "more than one input location", register: registerMultipartInput[struct {
			File File `form:"file" path:"file" json:"-"`
		}]},
		{name: "query conflict", want: "more than one input location", register: registerMultipartInput[struct {
			File File `form:"file" query:"file" json:"-"`
		}]},
		{name: "header conflict", want: "more than one input location", register: registerMultipartInput[struct {
			File File `form:"file" header:"file" json:"-"`
		}]},
		{name: "cookie conflict", want: "more than one input location", register: registerMultipartInput[struct {
			File File `form:"file" cookie:"file" json:"-"`
		}]},
		{name: "embedded file", want: "must not be embedded", register: registerMultipartInput[struct {
			File `form:"file" json:"-"`
		}]},
		{name: "schema tag", want: "must not have a schema tag", register: registerMultipartInput[struct {
			File File `form:"file" json:"-" schema:"nonempty"`
		}]},
		{name: "empty schema tag", want: "must not have a schema tag", register: registerMultipartInput[struct {
			File File `form:"file" json:"-" schema:""`
		}]},
	} {
		t.Run(tc.name, func(t *testing.T) { mustPanicWith(t, tc.want, tc.register) })
	}
	mustPanicWith(t, "takes no body", func() {
		Get(New(), "/upload", func(context.Context, struct {
			File File `form:"file" json:"-"`
		}) (NoContent, error) {
			return NoContent{}, nil
		})
	})
}

func TestMultipartMalformedOptionalFileDoesNotRunHandler(t *testing.T) {
	type input struct {
		Document *File `form:"document" json:"-"`
	}
	var calls atomic.Int32
	router := New()
	Post(router, "/upload", func(context.Context, input) (NoContent, error) {
		calls.Add(1)
		return NoContent{}, nil
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	body, _ := multipartTestBody(t, multipartTestPart{name: "document", filename: "file.txt"})
	status, data := multipartTestRequest(t, server, "/upload", body, "multipart/form-data; boundary=wrong", false)
	if calls.Load() != 0 || status != http.StatusBadRequest || !bytes.Contains(data, []byte(`"_tag":"DecodeError"`)) {
		t.Fatalf("calls=%d status=%d body=%s", calls.Load(), status, data)
	}
}
