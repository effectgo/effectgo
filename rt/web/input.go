package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/faintghost/effectgo/rt/schema"
)

var timeType = reflect.TypeFor[time.Time]()

// inputPlan describes how one handler's input is read. It is built at
// registration, where every mismatch between the input type, the method and
// the pattern panics.
type inputPlan[In any] struct {
	noInput            bool
	jsonBody           bool
	jsonBodyEmpty      bool
	jsonBodyBestEffort bool
	jsonBodyExact      bool
	filterJSONURL      bool
	formBody           bool
	urlType            reflect.Type
	readsBody          bool
	fields             []inputField
	urlKeys            map[string]bool // json names of fields supplied outside the body
	hasQuery           bool
	formFields         []formField
	formNames          map[string]int
}

type inputField struct {
	index      int
	fromPath   bool
	fromHeader bool
	fromCookie bool
	urlName    string // path wildcard, query parameter, header or cookie name
	jsonName   string // key in the object that schema.Decode reads
	typ        reflect.Type
}

func newInputPlan[In any](method, pattern string, jsonBody bool) (*inputPlan[In], error) {
	t := reflect.TypeFor[In]()
	if t == reflect.TypeFor[NoInput]() {
		if w := nameOfPattern(pattern); len(w) > 0 {
			return nil, fmt.Errorf("pattern %s has {%s}, but the input is NoInput", pattern, w[0])
		}
		return &inputPlan[In]{noInput: true}, nil
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("input %s must be a struct or web.NoInput", t)
	}
	p := &inputPlan[In]{
		jsonBody:  jsonBody,
		readsBody: method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete,
		urlKeys:   map[string]bool{},
	}
	p.filterJSONURL = !reflect.PointerTo(t).Implements(reflect.TypeFor[json.Unmarshaler]())
	wildcards := nameOfPattern(pattern)
	used := map[string]bool{}
	hasBody := false
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		jsonTag := f.Tag.Get("json")
		pathName, hasPath := f.Tag.Lookup("path")
		queryName, hasQuery := f.Tag.Lookup("query")
		headerName, hasHeader := f.Tag.Lookup("header")
		cookieName, hasCookie := f.Tag.Lookup("cookie")
		formName, hasForm := f.Tag.Lookup("form")
		if hasPath && hasQuery {
			return nil, fmt.Errorf("field %s has both path and query tags", f.Name)
		}
		locations := 0
		for _, present := range []bool{hasPath, hasQuery, hasHeader, hasCookie, hasForm} {
			if present {
				locations++
			}
		}
		if locations > 1 {
			return nil, fmt.Errorf("field %s has more than one input location tag", f.Name)
		}
		if hasForm {
			if f.Anonymous {
				return nil, fmt.Errorf("form field %s must not be embedded", f.Name)
			}
			if formName == "" {
				return nil, fmt.Errorf("form field %s has an empty name", f.Name)
			}
			isFile := f.Type == reflect.TypeFor[File]() || f.Type == reflect.TypeFor[*File]()
			jsonName, _, _ := strings.Cut(jsonTag, ",")
			if isFile {
				if jsonTag != "-" {
					return nil, fmt.Errorf("form field %s must have json:\"-\"", f.Name)
				}
				if _, hasSchema := f.Tag.Lookup("schema"); hasSchema {
					return nil, fmt.Errorf("form field %s must not have a schema tag", f.Name)
				}
			} else {
				typ := f.Type
				if typ.Kind() == reflect.Slice {
					typ = typ.Elem()
					if typ.Kind() == reflect.Pointer {
						return nil, fmt.Errorf("form field %s must not have pointer slice elements", f.Name)
					}
				}
				for typ.Kind() == reflect.Pointer {
					typ = typ.Elem()
				}
				if !scalar(typ) || typ == timeType {
					return nil, fmt.Errorf("form field %s must be web.File or *web.File, or a string, boolean or number scalar, got %s", f.Name, f.Type)
				}
				if jsonName != formName || jsonName == "-" {
					return nil, fmt.Errorf("scalar form field %s must have a matching JSON name", f.Name)
				}
			}
			if p.formNames == nil {
				p.formNames = map[string]int{}
			}
			if _, exists := p.formNames[formName]; exists {
				return nil, fmt.Errorf("duplicate form name %q", formName)
			}
			p.formNames[formName] = len(p.formFields)
			p.formFields = append(p.formFields, formField{name: formName, index: i, optional: f.Type.Kind() == reflect.Pointer, file: isFile, typ: f.Type})
			continue
		}
		if locations == 0 {
			if jsonTag != "-" {
				hasBody = true
			}
			continue
		}
		if jsonTag == "-" {
			return nil, fmt.Errorf("field %s has an input location tag but json:\"-\"", f.Name)
		}
		jsonName, _, _ := strings.Cut(jsonTag, ",")
		if jsonName == "" {
			jsonName = f.Name
		}
		if hasPath {
			if !contains(wildcards, pathName) {
				return nil, fmt.Errorf("path field %s: {%s} is not in pattern %s", f.Name, pathName, pattern)
			}
			if !scalar(f.Type) {
				return nil, fmt.Errorf("path field %s must be a scalar, got %s", f.Name, f.Type)
			}
			used[pathName] = true
			p.fields = append(p.fields, inputField{index: i, fromPath: true, urlName: pathName, jsonName: jsonName, typ: f.Type})
		} else {
			name := queryName
			if hasHeader {
				name = headerName
			} else if hasCookie {
				name = cookieName
			}
			if hasQuery && queryName == "" {
				return nil, fmt.Errorf("query field %s has an empty name", f.Name)
			}
			if name == "" {
				return nil, fmt.Errorf("field %s has an empty input location name", f.Name)
			}
			if !hasQuery && !scalar(f.Type) {
				return nil, fmt.Errorf("field %s must be a scalar, got %s", f.Name, f.Type)
			}
			if hasQuery && !scalar(f.Type) && !(f.Type.Kind() == reflect.Slice && scalar(f.Type.Elem())) {
				return nil, fmt.Errorf("query field %s must be a scalar or a slice of scalars, got %s", f.Name, f.Type)
			}
			p.hasQuery = p.hasQuery || hasQuery
			p.fields = append(p.fields, inputField{index: i, fromHeader: hasHeader, fromCookie: hasCookie, urlName: name, jsonName: jsonName, typ: f.Type})
		}
		p.urlKeys[jsonName] = true
	}
	if jsonBody {
		if len(p.fields) > 0 {
			fields := make([]reflect.StructField, len(p.fields))
			for i, field := range p.fields {
				fields[i] = t.Field(field.index)
				fields[i].Anonymous = false
			}
			p.urlType = reflect.StructOf(fields)
			if err := schema.CompileType(p.urlType); err != nil {
				return nil, fmt.Errorf("input URL fields: %w", err)
			}
		}
	} else if err := schema.Compile[In](); err != nil {
		return nil, fmt.Errorf("input: %w", err)
	}
	for _, w := range wildcards {
		if !used[w] {
			return nil, fmt.Errorf("pattern %s: {%s} has no path field", pattern, w)
		}
	}
	if len(p.formFields) > 0 {
		if jsonBody {
			return nil, fmt.Errorf("input cannot combine JSONBody with form fields")
		}
		if hasBody {
			return nil, fmt.Errorf("input cannot mix JSON body fields and form fields")
		}
		if !p.readsBody {
			return nil, fmt.Errorf("%s takes no body, but the input has form fields", method)
		}
	}
	if hasBody && !p.readsBody {
		return nil, fmt.Errorf("%s takes no body, but the input has body fields", method)
	}
	if !hasBody && len(p.formFields) == 0 {
		p.readsBody = false
	}
	return p, nil
}

// decode builds the object that schema.Decode reads: the body's fields (for
// methods that read a body), then the path and query values. A body key that
// names a path or query field is an unknown field, since the URL owns it.
func (p *inputPlan[In]) decode(w http.ResponseWriter, req *http.Request, maxBody int64) (In, error) {
	v, cleanup, err := p.decodeWithCleanup(w, req, maxBody)
	defer cleanup()
	return v, err
}

// decodeWithCleanup retains uploaded temporary files until cleanup is called.
func (p *inputPlan[In]) decodeWithCleanup(w http.ResponseWriter, req *http.Request, maxBody int64) (In, func(), error) {
	var paths []string
	cleanup := func() {
		for _, path := range paths {
			_ = os.Remove(path)
		}
	}
	v, err := p.decodeInput(w, req, maxBody, &paths)
	if err != nil {
		cleanup()
		p.attachIssueSources(err)
	}
	return v, cleanup, err
}

// Sources come from declared input fields, never from same-named URL values.
func (p *inputPlan[In]) attachIssueSources(err error) {
	de, ok := errors.AsType[*schema.DecodeError](err)
	if !ok {
		return
	}
	for i := range de.Issues {
		issue := &de.Issues[i]
		if issue.Source != "" {
			continue
		}
		issue.Source = "body"
		if len(issue.Path) == 0 {
			continue
		}
		for _, field := range p.fields {
			if issue.Path[0] != field.jsonName {
				continue
			}
			switch {
			case field.fromPath:
				issue.Source = "path"
			case field.fromHeader:
				issue.Source = "header"
			case field.fromCookie:
				issue.Source = "cookie"
			default:
				issue.Source = "query"
			}
			break
		}
	}
}

func (p *inputPlan[In]) decodeInput(w http.ResponseWriter, req *http.Request, maxBody int64, tempFiles *[]string) (In, error) {
	var zero In
	if p.noInput {
		return zero, nil
	}
	obj := map[string]json.RawMessage{}
	var collisions []schema.Issue
	var files map[string]File
	var bodyValue In
	if len(p.formFields) > 0 {
		var err error
		files, err = p.decodeMultipart(w, req, maxBody, obj, tempFiles)
		if err != nil {
			return zero, err
		}
	}
	if p.readsBody && len(p.formFields) == 0 {
		if p.jsonBody {
			var err error
			bodyValue, err = p.decodeJSONBody(w, req, maxBody)
			if err != nil {
				return zero, err
			}
		} else {
			body, err := readBody(w, req, maxBody)
			if err != nil {
				return zero, err
			}
			bodyObj, err := bodyObject(body)
			if err != nil {
				return zero, err
			}
			for k, v := range bodyObj {
				if p.urlKeys[k] {
					collisions = append(collisions, schema.Issue{
						Path: []string{k}, Code: schema.CodeUnknown, Message: "field is set from the URL, not the body",
						Source: "body", Value: v,
					})
					continue
				}
				obj[k] = v
			}
		}
	}
	var query map[string][]string
	if p.hasQuery {
		query = req.URL.Query()
	}
	urlValues := make(map[string]any, len(p.fields))
	for _, f := range p.fields {
		if f.fromPath {
			obj[f.jsonName] = scalarJSON(f.typ, req.PathValue(f.urlName))
			urlValues[f.jsonName] = req.PathValue(f.urlName)
			continue
		}
		if f.fromHeader {
			if value := req.Header.Get(f.urlName); value != "" {
				obj[f.jsonName] = scalarJSON(f.typ, value)
				urlValues[f.jsonName] = value
			}
			continue
		}
		if f.fromCookie {
			cookie, err := req.Cookie(f.urlName)
			if err == nil {
				obj[f.jsonName] = scalarJSON(f.typ, cookie.Value)
				urlValues[f.jsonName] = cookie.Value
			}
			continue
		}
		vals, ok := query[f.urlName]
		if !ok {
			continue
		}
		if f.typ.Kind() == reflect.Slice {
			obj[f.jsonName] = sliceJSON(f.typ.Elem(), vals)
			urlValues[f.jsonName] = vals
		} else {
			obj[f.jsonName] = scalarJSON(f.typ, vals[0])
			urlValues[f.jsonName] = vals[0]
		}
	}
	// obj holds only values this function built or that json.Valid accepted,
	// so a failure here is a programming error.
	buf, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	if p.jsonBody {
		v, err := p.mergeJSONURL(bodyValue, buf)
		attachURLIssueValues(err, urlValues)
		return v, err
	}
	v, err := schema.Decode[In](buf)
	attachURLIssueValues(err, urlValues)
	if len(collisions) > 0 {
		var de *schema.DecodeError
		if errors.As(err, &de) {
			de.Issues = append(collisions, de.Issues...)
			return zero, de
		}
		return zero, &schema.DecodeError{Issues: collisions}
	}
	if err != nil {
		return zero, err
	}
	if len(p.formFields) > 0 {
		dst := reflect.ValueOf(&v).Elem()
		for _, f := range p.formFields {
			if !f.file {
				continue
			}
			file, present := files[f.name]
			if !present {
				continue
			}
			if f.optional {
				dst.Field(f.index).Set(reflect.ValueOf(&file))
			} else {
				dst.Field(f.index).Set(reflect.ValueOf(file))
			}
		}
	}
	return v, nil
}

func attachURLIssueValues(err error, values map[string]any) {
	de, ok := errors.AsType[*schema.DecodeError](err)
	if !ok {
		return
	}
	for i := range de.Issues {
		issue := &de.Issues[i]
		// Validation issues already retain the JSON value consumed by schema.
		// Only conversion failures need the original transport string.
		if issue.Code != schema.CodeType || len(issue.Path) == 0 {
			continue
		}
		if value, present := values[issue.Path[0]]; present {
			if items, ok := value.([]string); ok && len(issue.Path) > 1 {
				if index, err := strconv.Atoi(issue.Path[1]); err == nil && index >= 0 && index < len(items) {
					value = items[index]
				}
			}
			issue.Value = value
		}
	}
}

func (p *inputPlan[In]) decodeJSONBody(w http.ResponseWriter, req *http.Request, maxBody int64) (In, error) {
	var v In
	filter := p.filterJSONURL && len(p.fields) > 0 && !p.jsonBodyBestEffort
	var raw json.RawMessage
	var target any = &v
	if filter {
		target = &raw
	}
	if p.jsonBodyExact {
		body, err := readBody(w, req, maxBody)
		if err != nil {
			return v, err
		}
		if err := json.Unmarshal(body, target); err != nil {
			return v, multipartError("", schema.CodeSyntax, err.Error())
		}
	} else {
		reader := &jsonInputReader{Reader: http.MaxBytesReader(w, req.Body, maxBody)}
		decoder := json.NewDecoder(reader)
		err := decoder.Decode(target)
		if err != nil && reader.err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](reader.err); ok {
				return v, BodyTooLarge{}
			}
			return v, multipartError("", schema.CodeSyntax, reader.err.Error())
		}
		if p.jsonBodyBestEffort || (p.jsonBodyEmpty && err == io.EOF && decoder.InputOffset() == 0) {
			return v, nil
		}
		if err != nil {
			return v, multipartError("", schema.CodeSyntax, err.Error())
		}
	}
	if filter {
		body, err := p.filterURLBody(raw)
		if err == nil {
			err = json.Unmarshal(body, &v)
		}
		if err != nil {
			return v, multipartError("", schema.CodeSyntax, err.Error())
		}
	}
	return v, nil
}

// Preserve duplicate body keys and nested decoder bytes while excluding only
// top-level URL-owned fields. Root custom decoders receive the original body.
func (p *inputPlan[In]) filterURLBody(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || raw[0] != '{' {
		return raw, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	body := make([]byte, 1, len(raw))
	body[0] = '{'
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name := token.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		owned := false
		for key := range p.urlKeys {
			if strings.EqualFold(name, key) {
				owned = true
				break
			}
		}
		if owned {
			continue
		}
		if len(body) > 1 {
			body = append(body, ',')
		}
		key, _ := json.Marshal(name)
		body = append(body, key...)
		body = append(body, ':')
		body = append(body, value...)
	}
	return append(body, '}'), nil
}

// Track transport failures separately from custom JSON unmarshal errors.
type jsonInputReader struct {
	io.Reader
	err error
}

func (r *jsonInputReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}

func (p *inputPlan[In]) mergeJSONURL(v In, buf []byte) (In, error) {
	if p.urlType == nil {
		return v, nil
	}
	url := reflect.New(p.urlType)
	if err := schema.DecodeInto(buf, url.Interface()); err != nil {
		var zero In
		return zero, err
	}
	dst := reflect.ValueOf(&v).Elem()
	for i, field := range p.fields {
		dst.Field(field.index).Set(url.Elem().Field(i))
	}
	return v, nil
}

// scalarJSON turns a path or query string into the JSON value a field of type
// t expects. A string that does not fit the type is sent quoted, so that
// schema reports the type issue at the field.
func scalarJSON(t reflect.Type, s string) json.RawMessage {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t != timeType {
		switch t.Kind() {
		case reflect.Bool:
			if s == "true" || s == "false" {
				return json.RawMessage(s)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return json.RawMessage(strconv.FormatInt(n, 10))
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if n, err := strconv.ParseUint(s, 10, 64); err == nil {
				return json.RawMessage(strconv.FormatUint(n, 10))
			}
		case reflect.Float32, reflect.Float64:
			if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				return json.RawMessage(strconv.FormatFloat(f, 'g', -1, 64))
			}
		}
	}
	b, _ := json.Marshal(s)
	return b
}

func sliceJSON(elem reflect.Type, vals []string) json.RawMessage {
	out := []byte{'['}
	for i, v := range vals {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, scalarJSON(elem, v)...)
	}
	return append(out, ']')
}

// scalar reports whether a field of type t is read from one string: a bool,
// a number, a string or a time, behind any number of pointers.
func scalar(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == timeType {
		return true
	}
	switch t.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
