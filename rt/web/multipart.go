package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"

	"github.com/faintghost/effectgo/rt/schema"
)

const multipartMemoryLimit int64 = 32 << 20

// File is one uploaded file. Tag each input field form:"name" json:"-";
// File requires the part, while *File permits its absence. Scalar form fields
// use matching form and JSON names, schema validation, optional pointers and
// repeated scalar slices. JSON body fields cannot accompany form fields.
// Scalar values and repeated-value bookkeeping have a separate 10 MiB budget.
// Uploads share a 32 MiB memory budget, then spill to temporary files. Open is
// valid until the handler and response complete; close each reader it returns.
// WithMaxBody bounds the entire multipart request, including trailing bytes.
type File struct {
	Filename    string
	ContentType string
	Size        int64
	Data        []byte // In-memory files only; use Open for either storage mode.
	path        string
}

// Open opens a fresh reader at the beginning of the uploaded file.
func (f File) Open() (io.ReadCloser, error) {
	if f.path != "" {
		return os.Open(f.path)
	}
	return io.NopCloser(bytes.NewReader(f.Data)), nil
}

type formField struct {
	name     string
	index    int
	optional bool
	file     bool
	typ      reflect.Type
}

func (p *inputPlan[In]) decodeMultipart(w http.ResponseWriter, req *http.Request, maxBody int64, obj map[string]json.RawMessage, tempFiles *[]string) (map[string]File, error) {
	if req.ContentLength > maxBody {
		return nil, BodyTooLarge{}
	}
	req.Body = http.MaxBytesReader(w, req.Body, maxBody)
	// Multipart readers may stop at the closing boundary. Drain the bounded
	// body even on malformed input so epilogues count toward the total limit.
	finish := func(err error) error {
		_, drainErr := io.Copy(io.Discard, req.Body)
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return BodyTooLarge{}
		}
		if _, ok := errors.AsType[*http.MaxBytesError](drainErr); ok {
			return BodyTooLarge{}
		}
		if err != nil {
			return err
		}
		if drainErr != nil {
			return multipartError("", schema.CodeSyntax, "cannot read multipart body")
		}
		return nil
	}
	fail := func(name, code, message string) (map[string]File, error) {
		return nil, finish(multipartError(name, code, message))
	}
	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return fail("", schema.CodeSyntax, "expected multipart/form-data with a boundary")
	}
	reader := multipart.NewReader(req.Body, params["boundary"])
	files := make(map[string]File, len(p.formFields))
	seen := make(map[string]bool, len(p.formFields))
	repeated := make(map[string][]string)
	memoryLeft := multipartMemoryLimit
	valueMemoryLeft := int64(10 << 20)
	for partCount := 0; ; partCount++ {
		part, err := reader.NextRawPart()
		// Only an exact EOF marks a final boundary; wrapped EOF is malformed.
		if err == io.EOF {
			break
		}
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				return nil, finish(err)
			}
			return fail("", schema.CodeSyntax, "malformed multipart body")
		}
		if p.formBody && partCount >= 1000 {
			return fail("", schema.CodeType, "too many multipart parts")
		}
		name := part.FormName()
		index, known := p.formNames[name]
		var field formField
		if known {
			field = p.formFields[index]
		}
		if p.formBody && (!known || seen[name] || field.file != (part.FileName() != "")) {
			if _, err := io.Copy(io.Discard, part); err != nil {
				if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
					return nil, finish(err)
				}
				return fail(name, schema.CodeSyntax, "cannot read form part")
			}
			continue
		}
		if name == "" {
			return fail("", schema.CodeType, "expected a named form part")
		}
		if !known {
			return fail(name, schema.CodeUnknown, "unknown form field")
		}
		isSlice := field.typ.Kind() == reflect.Slice
		if seen[name] && !isSlice {
			return fail(name, schema.CodeType, "duplicate form part")
		}
		seen[name] = true
		_, disposition, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		_, hasFilename := disposition["filename"]
		if field.file && part.FileName() == "" {
			return fail(name, schema.CodeType, "expected a named file part")
		}
		if !field.file && hasFilename && !p.formBody {
			return fail(name, schema.CodeType, "expected a scalar form part")
		}
		remaining := memoryLeft
		if !field.file {
			remaining = valueMemoryLeft
		}
		data, err := io.ReadAll(io.LimitReader(part, remaining+1))
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				return nil, finish(err)
			}
			return fail(name, schema.CodeSyntax, "cannot read form part")
		}
		if field.file {
			file := File{Filename: part.FileName(), ContentType: part.Header.Get("Content-Type"), Size: int64(len(data))}
			if int64(len(data)) <= memoryLeft {
				file.Data = data
				memoryLeft -= int64(len(data))
			} else {
				temp, err := os.CreateTemp("", "effectgo-upload-*")
				if err != nil {
					return nil, finish(err)
				}
				*tempFiles = append(*tempFiles, temp.Name())
				file.path = temp.Name()
				_, writeErr := temp.Write(data)
				var copied int64
				if writeErr == nil {
					copied, writeErr = io.Copy(temp, part)
				}
				closeErr := temp.Close()
				if writeErr != nil {
					if errors.Is(writeErr, io.ErrUnexpectedEOF) {
						return fail(name, schema.CodeSyntax, "cannot read form part")
					}
					return nil, finish(writeErr)
				}
				if closeErr != nil {
					return nil, finish(closeErr)
				}
				file.Size += copied
			}
			files[name] = file
		} else {
			// Include per-value bookkeeping, so repeated empty parts also have
			// a finite memory budget under deliberately large body limits.
			cost := max(int64(len(data)), 64)
			if cost > valueMemoryLeft {
				return fail(name, schema.CodeType, "form values exceed memory limit")
			}
			valueMemoryLeft -= cost
			if isSlice {
				repeated[name] = append(repeated[name], string(data))
			} else {
				obj[name] = scalarJSON(field.typ, string(data))
			}
		}
	}
	if err := finish(nil); err != nil {
		return nil, err
	}
	var issues []schema.Issue
	for _, field := range p.formFields {
		if !field.file {
			if values, present := repeated[field.name]; present {
				obj[field.name] = sliceJSON(field.typ.Elem(), values)
			}
			continue
		}
		if !seen[field.name] && !field.optional {
			issues = append(issues, schema.Issue{Path: []string{field.name}, Code: schema.CodeMissing, Message: "required file is missing"})
		}
	}
	if len(issues) > 0 {
		return nil, &schema.DecodeError{Issues: issues}
	}
	return files, nil
}

func multipartError(name, code, message string) error {
	path := []string{}
	if name != "" {
		path = append(path, name)
	}
	return &schema.DecodeError{Issues: []schema.Issue{{Path: path, Code: code, Message: message}}}
}
