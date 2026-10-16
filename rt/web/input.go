package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
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
	noInput   bool
	readsBody bool
	fields    []inputField
	urlKeys   map[string]bool // json names of fields supplied outside the body
	hasQuery  bool
}

type inputField struct {
	fromPath   bool
	fromHeader bool
	fromCookie bool
	urlName    string // path wildcard, query parameter, header or cookie name
	jsonName   string // key in the object that schema.Decode reads
	typ        reflect.Type
}

func newInputPlan[In any](method, pattern string) (*inputPlan[In], error) {
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
	if err := schema.Compile[In](); err != nil {
		return nil, fmt.Errorf("input: %w", err)
	}
	p := &inputPlan[In]{
		readsBody: method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch,
		urlKeys:   map[string]bool{},
	}
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
		if hasPath && hasQuery {
			return nil, fmt.Errorf("field %s has both path and query tags", f.Name)
		}
		locations := 0
		for _, present := range []bool{hasPath, hasQuery, hasHeader, hasCookie} {
			if present {
				locations++
			}
		}
		if locations > 1 {
			return nil, fmt.Errorf("field %s has more than one input location tag", f.Name)
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
			p.fields = append(p.fields, inputField{fromPath: true, urlName: pathName, jsonName: jsonName, typ: f.Type})
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
			p.fields = append(p.fields, inputField{fromHeader: hasHeader, fromCookie: hasCookie, urlName: name, jsonName: jsonName, typ: f.Type})
		}
		p.urlKeys[jsonName] = true
	}
	for _, w := range wildcards {
		if !used[w] {
			return nil, fmt.Errorf("pattern %s: {%s} has no path field", pattern, w)
		}
	}
	if hasBody && !p.readsBody {
		return nil, fmt.Errorf("%s takes no body, but the input has body fields", method)
	}
	return p, nil
}

// decode builds the object that schema.Decode reads: the body's fields (for
// methods that read a body), then the path and query values. A body key that
// names a path or query field is an unknown field, since the URL owns it.
func (p *inputPlan[In]) decode(w http.ResponseWriter, req *http.Request, maxBody int64) (In, error) {
	var zero In
	if p.noInput {
		return zero, nil
	}
	obj := map[string]json.RawMessage{}
	var collisions []schema.Issue
	if p.readsBody {
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
				})
				continue
			}
			obj[k] = v
		}
	}
	var query map[string][]string
	if p.hasQuery {
		query = req.URL.Query()
	}
	for _, f := range p.fields {
		if f.fromPath {
			obj[f.jsonName] = scalarJSON(f.typ, req.PathValue(f.urlName))
			continue
		}
		if f.fromHeader {
			if value := req.Header.Get(f.urlName); value != "" {
				obj[f.jsonName] = scalarJSON(f.typ, value)
			}
			continue
		}
		if f.fromCookie {
			cookie, err := req.Cookie(f.urlName)
			if err == nil {
				obj[f.jsonName] = scalarJSON(f.typ, cookie.Value)
			}
			continue
		}
		vals, ok := query[f.urlName]
		if !ok {
			continue
		}
		if f.typ.Kind() == reflect.Slice {
			obj[f.jsonName] = sliceJSON(f.typ.Elem(), vals)
		} else {
			obj[f.jsonName] = scalarJSON(f.typ, vals[0])
		}
	}
	// obj holds only values this function built or that json.Valid accepted,
	// so a failure here is a programming error.
	buf, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	v, err := schema.Decode[In](buf)
	if len(collisions) > 0 {
		var de *schema.DecodeError
		if errors.As(err, &de) {
			de.Issues = append(collisions, de.Issues...)
			return zero, de
		}
		return zero, &schema.DecodeError{Issues: collisions}
	}
	return v, err
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
