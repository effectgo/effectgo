// Package schema decodes one JSON value into a Go struct. Decoding is strict:
// unknown fields, missing required fields, JSON values of the wrong type and
// values that break a schema tag are all reported, and every issue is
// collected rather than stopping at the first one.
//
// Field names come from the json tag. Validation comes from the schema tag,
// whose vocabulary is closed: trim, nonempty, singleline, email, min=N,
// max=N and pattern=NAME (a name registered with RegisterPattern). A
// non-pointer field is required; a pointer field is optional and JSON null
// counts as absent. A type that implements Enum is an enum: a decoded value
// for which Valid returns false is reported with code "enum".
//
// The analysis of each type is computed once and cached. Compile reports
// unsupported types and malformed tags without decoding anything; Decode
// calls it first.
//
//effectgo:package pure
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Issue codes. The set is closed; every Issue carries one of these.
const (
	CodeSyntax     = "syntax"     // the input is not exactly one JSON value
	CodeMissing    = "missing"    // a required field is absent
	CodeUnknown    = "unknown"    // an object key is not a field of the struct
	CodeType       = "type"       // a JSON value of the wrong kind or range
	CodeMin        = "min"        // below min=N
	CodeMax        = "max"        // above max=N
	CodeNonEmpty   = "nonempty"   // empty after trimming (or zero length)
	CodeSingleLine = "singleline" // contains \r or \n
	CodeEmail      = "email"      // not a bare email address
	CodePattern    = "pattern"    // does not match the registered pattern
	CodeEnum       = "enum"       // Valid() reported false
)

// Issue is one problem found while decoding. Path names the field, or the
// slice index or map key, from the root of the value; it is empty for the
// root itself.
type Issue struct {
	Path    []string `json:"path"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	// Source is the declared HTTP location (path, query, header, cookie or body).
	// It is empty for generic schema decoding. Value is the rejected value:
	// schema decoding retains an owned json.RawMessage; HTTP conversion
	// failures retain the original string (or repeated strings).
	// Missing values are nil. Both fields are excluded from default JSON.
	Source string `json:"-"`
	Value  any    `json:"-"`
}

// DecodeError lists every issue found while decoding one value. It is a
// client error: StatusCode is 400.
type DecodeError struct {
	Issues []Issue `json:"issues"`
}

// StatusCode reports 400, the status of a value that fails decoding.
func (e DecodeError) StatusCode() int { return 400 }

// Error summarises the issues; the first one is named in full.
func (e DecodeError) Error() string {
	if len(e.Issues) == 0 {
		return "schema: decode failed"
	}
	first := e.Issues[0]
	return fmt.Sprintf("schema: %d issue(s); first: %s: %s", len(e.Issues), pathString(first.Path), first.Message)
}

// RegisterPattern makes a regular expression available to pattern=NAME.
// Registration belongs in init: a type that uses a name is compiled against
// the registry at its first use, and a duplicate name panics.
//
// The registry is the one global state of the package: RegisterPattern writes
// the package-level patterns map under a lock. It is meant to run once, from
// init, so the function allows global and panic and nothing else.
//
//effectgo:allow global,panic
func RegisterPattern(name string, re *regexp.Regexp) {
	if name == "" || strings.ContainsAny(name, ",=") {
		panic(fmt.Sprintf("schema: invalid pattern name %q", name))
	}
	if re == nil {
		panic(fmt.Sprintf("schema: pattern %q is nil", name))
	}
	patternsMu.Lock()
	defer patternsMu.Unlock()
	if _, dup := patterns[name]; dup {
		panic(fmt.Sprintf("schema: pattern %q registered twice", name))
	}
	patterns[name] = re
}

var (
	patternsMu sync.RWMutex
	patterns   = map[string]*regexp.Regexp{}
)

func lookupPattern(name string) (*regexp.Regexp, bool) {
	patternsMu.RLock()
	defer patternsMu.RUnlock()
	re, ok := patterns[name]
	return re, ok
}

// Enum is implemented by an enum type: a type whose decoded values must be
// among its valid ones. Decode calls Valid on every decoded value of such a
// type, and reports the value with code "enum" when Valid returns false.
//
// Valid is the only user code that Decode runs, so it is declared pure: a
// Valid that reads the clock, the network or a global breaks that contract,
// and effectgo reports the implementation.
type Enum interface {
	//effectgo:pure
	Valid() bool
}

var (
	timeType       = reflect.TypeFor[time.Time]()
	rawMessageType = reflect.TypeFor[json.RawMessage]()
	enumType       = reflect.TypeFor[Enum]()
)

// Decode strictly decodes data, which must hold exactly one JSON value, into
// a T. On failure it returns the zero T and a *DecodeError listing every
// issue. If T cannot be compiled (a bad tag or an unsupported type) Decode
// panics with Compile's error, as regexp.MustCompile does: that is a
// programming error, not a property of the input, so it is not in the error
// result.
//
// The panic is deliberate, as regexp.MustCompile's is: it is raised for a type
// that Compile rejects.
//
//effectgo:allow panic
func Decode[T any](data []byte) (T, error) {
	var out T
	err := DecodeInto(data, &out)
	return out, err
}

// DecodeInto strictly decodes data into a nonnil pointer. It applies the same
// rules and enum checks as Decode, and resets dst to zero on failure.
// Like Decode, it panics if the destination type cannot be compiled.
//
//effectgo:allow panic
func DecodeInto(data []byte, dst any) error {
	target := reflect.ValueOf(dst)
	if !target.IsValid() || target.Kind() != reflect.Pointer || target.IsNil() {
		return fmt.Errorf("schema: destination must be a nonnil pointer")
	}
	t := target.Type().Elem()
	if err := CompileType(t); err != nil {
		panic(err)
	}
	v := target.Elem()
	v.SetZero()
	if !json.Valid(data) {
		var v any
		err := json.Unmarshal(data, &v)
		return &DecodeError{Issues: []Issue{{Path: []string{}, Code: CodeSyntax, Message: err.Error()}}}
	}
	d := &decoder{}
	d.decode(v, data, nil)
	if len(d.issues) > 0 {
		v.SetZero()
		return &DecodeError{Issues: d.issues}
	}
	return nil
}

// Compile checks that T can be decoded: every field type is supported, every
// schema tag is in the vocabulary and fits its field type, and every named
// pattern is registered. A successful check is cached.
func Compile[T any]() error {
	return CompileType(reflect.TypeFor[T]())
}

// CompileType checks and caches a runtime type using the same plans as Compile.
func CompileType(t reflect.Type) error {
	if t == nil {
		return fmt.Errorf("schema: type must not be nil")
	}
	if _, ok := compiled.Load(t); ok {
		return nil
	}
	if err := checkType(t, map[reflect.Type]bool{}); err != nil {
		return fmt.Errorf("schema: %s: %w", t, err)
	}
	compiled.Store(t, true)
	return nil
}

var compiled sync.Map // reflect.Type -> true

// checkType walks every type reachable from t. seen stops recursion through
// recursive types; struct plans are validated by planFor.
func checkType(t reflect.Type, seen map[reflect.Type]bool) error {
	switch t.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return nil
	case reflect.Pointer:
		return checkType(t.Elem(), seen)
	case reflect.Slice:
		if t == rawMessageType {
			return nil
		}
		if t.Elem().Kind() == reflect.Uint8 {
			return fmt.Errorf("byte slices are not supported; use string or json.RawMessage")
		}
		return checkType(t.Elem(), seen)
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return fmt.Errorf("map key must be a string kind, got %s", t.Key())
		}
		return checkType(t.Elem(), seen)
	case reflect.Struct:
		if t == timeType || seen[t] {
			return nil
		}
		seen[t] = true
		p, err := planFor(t)
		if err != nil {
			return err
		}
		for _, f := range p.fields {
			if err := checkType(f.typ, seen); err != nil {
				return fmt.Errorf("field %s: %w", f.name, err)
			}
		}
		return nil
	case reflect.Interface:
		if t.NumMethod() == 0 {
			return nil
		}
		return fmt.Errorf("non-empty interface %s is not supported", t)
	}
	return fmt.Errorf("type %s is not supported", t)
}

// decoder carries the state of one Decode call: the path to the value being
// decoded (a stack, so valid input allocates no paths) and the issues so far.
type decoder struct {
	path   []string
	issues []Issue
}

func (d *decoder) add(code, msg string, raw []byte) {
	var value any
	if raw != nil {
		value = json.RawMessage(bytes.Clone(raw))
	}
	d.issues = append(d.issues, Issue{Path: copyPath(d.path), Code: code, Message: msg, Value: value})
}

// decode decodes raw into dst, which must be settable, and applies rules and
// the enum check. A value whose own JSON shape is wrong gets no rule or enum
// check; a container whose children have issues still gets its own rules.
//
// It allows panic, which mustPlan raises through structValue.
//
//effectgo:allow panic
func (d *decoder) decode(dst reflect.Value, raw []byte, rules []rule) {
	raw = bytes.TrimSpace(raw)
	t := dst.Type()
	if t.Kind() == reflect.Pointer {
		if isNull(raw) {
			return
		}
		p := reflect.New(t.Elem())
		d.decode(p.Elem(), raw, rules)
		dst.Set(p)
		return
	}
	if t == rawMessageType || t.Kind() == reflect.Interface {
		// Both accept any JSON value, null included.
		d.scalar(dst, raw)
		return
	}
	if isNull(raw) {
		d.add(CodeType, "expected "+describe(t)+", got null", raw)
		return
	}
	var ok bool
	switch {
	case t == timeType:
		ok = d.scalar(dst, raw)
	case t.Kind() == reflect.Struct:
		ok = d.structValue(dst, raw)
	case t.Kind() == reflect.Slice:
		ok = d.sliceValue(dst, raw)
	case t.Kind() == reflect.Map:
		ok = d.mapValue(dst, raw)
	default:
		ok = d.scalar(dst, raw)
	}
	if !ok {
		return
	}
	for _, r := range rules {
		if r.kind == ruleTrim {
			dst.SetString(strings.TrimSpace(dst.String()))
		}
	}
	for _, r := range rules {
		if r.kind == ruleTrim {
			continue
		}
		if code, msg, failed := r.check(dst); failed {
			d.add(code, msg, raw)
		}
	}
	d.enumCheck(dst, raw)
}

// scalar decodes a value whose Go type json.Unmarshal checks exactly: numbers
// must fit, strings must be strings, and so on.
func (d *decoder) scalar(dst reflect.Value, raw []byte) bool {
	if err := json.Unmarshal(raw, dst.Addr().Interface()); err != nil {
		if dst.Type() == timeType {
			d.add(CodeType, "expected RFC 3339 time, got "+jsonKind(raw), raw)
		} else {
			d.add(CodeType, "expected "+describe(dst.Type())+", got "+jsonKind(raw), raw)
		}
		return false
	}
	return true
}

// structValue reaches mustPlan (panic) through its own call, so it allows panic.
//
//effectgo:allow panic
func (d *decoder) structValue(dst reflect.Value, raw []byte) bool {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		d.add(CodeType, "expected object, got "+jsonKind(raw), raw)
		return false
	}
	p := mustPlan(dst.Type())
	for _, f := range p.fields {
		v, ok := obj[f.name]
		if !ok {
			if !f.optional() {
				d.path = append(d.path, f.name)
				d.add(CodeMissing, "required field is missing", nil)
				d.path = d.path[:len(d.path)-1]
			}
			continue
		}
		d.path = append(d.path, f.name)
		d.decode(dst.Field(f.index), v, f.rules)
		d.path = d.path[:len(d.path)-1]
	}
	var unknown []string
	for k := range obj {
		if _, ok := p.byName[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		d.path = append(d.path, k)
		d.add(CodeUnknown, "unknown field", obj[k])
		d.path = d.path[:len(d.path)-1]
	}
	return true
}

// sliceValue decodes its elements through decode, so it allows what decode
// allows (panic, see decode).
//
//effectgo:allow panic
func (d *decoder) sliceValue(dst reflect.Value, raw []byte) bool {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		d.add(CodeType, "expected array, got "+jsonKind(raw), raw)
		return false
	}
	s := reflect.MakeSlice(dst.Type(), len(items), len(items))
	for i, item := range items {
		d.path = append(d.path, strconv.Itoa(i))
		d.decode(s.Index(i), item, nil)
		d.path = d.path[:len(d.path)-1]
	}
	dst.Set(s)
	return true
}

// mapValue decodes its values through decode, so it allows what decode allows
// (panic, see decode).
//
//effectgo:allow panic
func (d *decoder) mapValue(dst reflect.Value, raw []byte) bool {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		d.add(CodeType, "expected object, got "+jsonKind(raw), raw)
		return false
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t := dst.Type()
	m := reflect.MakeMapWithSize(t, len(obj))
	for _, k := range keys {
		v := reflect.New(t.Elem()).Elem()
		d.path = append(d.path, k)
		d.decode(v, obj[k], nil)
		d.path = d.path[:len(d.path)-1]
		m.SetMapIndex(reflect.ValueOf(k).Convert(t.Key()), v)
	}
	dst.Set(m)
	return true
}

// enumCheck calls Valid on a decoded value whose type implements Enum, through
// the interface, whose method is pure by contract (see Enum).
func (d *decoder) enumCheck(dst reflect.Value, raw []byte) {
	t := dst.Type()
	switch {
	case t.Implements(enumType):
		if !dst.Interface().(Enum).Valid() {
			d.add(CodeEnum, "unknown value", raw)
		}
	case reflect.PointerTo(t).Implements(enumType):
		if !dst.Addr().Interface().(Enum).Valid() {
			d.add(CodeEnum, "unknown value", raw)
		}
	}
}

func isNull(raw []byte) bool { return string(raw) == "null" }

func copyPath(p []string) []string {
	out := make([]string, len(p))
	copy(out, p)
	return out
}

func pathString(p []string) string {
	if len(p) == 0 {
		return "(root)"
	}
	return strings.Join(p, ".")
}

// describe names the JSON shape a Go type expects, for issue messages.
func describe(t reflect.Type) string {
	switch {
	case t == timeType:
		return "RFC 3339 time"
	case t.Kind() == reflect.Bool:
		return "boolean"
	case t.Kind() == reflect.String:
		return "string"
	case t.Kind() >= reflect.Int && t.Kind() <= reflect.Uint64:
		return "integer"
	case t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64:
		return "number"
	case t.Kind() == reflect.Struct, t.Kind() == reflect.Map:
		return "object"
	case t.Kind() == reflect.Slice:
		return "array"
	}
	return "value"
}

// jsonKind names the JSON value raw holds, for issue messages.
func jsonKind(raw []byte) string {
	switch raw[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	}
	s := string(raw)
	if len(s) > 32 {
		s = s[:32] + "..."
	}
	return "number " + s
}

// ---- tags and plans ----

type ruleKind int

const (
	ruleTrim ruleKind = iota
	ruleNonEmpty
	ruleSingleLine
	ruleEmail
	rulePattern
	ruleMin
	ruleMax
)

// rule is one parsed entry of a schema tag. The numeric argument is kept in
// the form the field's kind compares in; arg is the text for messages.
type rule struct {
	kind ruleKind
	arg  string
	ival int64
	uval uint64
	fval float64
	re   *regexp.Regexp
	name string
}

// check applies one rule other than trim to a decoded value.
func (r rule) check(v reflect.Value) (code, msg string, failed bool) {
	switch r.kind {
	case ruleMin, ruleMax:
		less, greater := r.compare(v)
		if r.kind == ruleMin && less {
			return CodeMin, "must be at least " + r.arg, true
		}
		if r.kind == ruleMax && greater {
			return CodeMax, "must be at most " + r.arg, true
		}
	case ruleNonEmpty:
		if v.Len() == 0 {
			return CodeNonEmpty, "must not be empty", true
		}
	case ruleSingleLine:
		if strings.ContainsAny(v.String(), "\r\n") {
			return CodeSingleLine, "must be a single line", true
		}
	case ruleEmail:
		if !validEmail(v.String()) {
			return CodeEmail, "must be an email address", true
		}
	case rulePattern:
		if !r.re.MatchString(v.String()) {
			return CodePattern, "does not match pattern " + r.name, true
		}
	}
	return "", "", false
}

// compare reports whether v is below or above the rule's bound. Numbers are
// compared by value; strings by rune count; slices and maps by length.
func (r rule) compare(v reflect.Value) (less, greater bool) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := v.Int()
		return n < r.ival, n > r.ival
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := v.Uint()
		return n < r.uval, n > r.uval
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		return f < r.fval, f > r.fval
	case reflect.String:
		n := int64(utf8.RuneCountInString(v.String()))
		return n < r.ival, n > r.ival
	}
	n := int64(v.Len())
	return n < r.ival, n > r.ival
}

// category is the kind of a field as far as schema tags are concerned.
type category int

const (
	catNone category = iota
	catString
	catSlice
	catMap
	catInt
	catUint
	catFloat
)

func categoryOf(t reflect.Type) category {
	if t == timeType || t == rawMessageType {
		return catNone
	}
	switch t.Kind() {
	case reflect.String:
		return catString
	case reflect.Slice:
		return catSlice
	case reflect.Map:
		return catMap
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return catInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return catUint
	case reflect.Float32, reflect.Float64:
		return catFloat
	}
	return catNone
}

// parseRules parses a schema tag for a field of type t. Each rule must apply
// to the field's kind; the tag may not repeat a rule.
func parseRules(tag string, t reflect.Type) ([]rule, error) {
	if tag == "" {
		return nil, nil
	}
	base := t
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	cat := categoryOf(base)
	var rules []rule
	seen := map[string]bool{}
	for _, item := range strings.Split(tag, ",") {
		item = strings.TrimSpace(item)
		name, arg, hasArg := strings.Cut(item, "=")
		if seen[name] {
			return nil, fmt.Errorf("rule %q is repeated", name)
		}
		seen[name] = true
		r, err := parseRule(name, arg, hasArg, cat, base)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func parseRule(name, arg string, hasArg bool, cat category, base reflect.Type) (rule, error) {
	noArg := func() error {
		if hasArg {
			return fmt.Errorf("rule %q takes no argument", name)
		}
		return nil
	}
	needString := func() error {
		if cat != catString {
			return fmt.Errorf("rule %q does not apply to %s", name, base)
		}
		return nil
	}
	switch name {
	case "trim", "singleline", "email":
		if err := noArg(); err != nil {
			return rule{}, err
		}
		if err := needString(); err != nil {
			return rule{}, err
		}
		kind := ruleTrim
		switch name {
		case "singleline":
			kind = ruleSingleLine
		case "email":
			kind = ruleEmail
		}
		return rule{kind: kind}, nil
	case "nonempty":
		if err := noArg(); err != nil {
			return rule{}, err
		}
		if cat != catString && cat != catSlice && cat != catMap {
			return rule{}, fmt.Errorf("rule %q does not apply to %s", name, base)
		}
		return rule{kind: ruleNonEmpty}, nil
	case "pattern":
		if err := needString(); err != nil {
			return rule{}, err
		}
		if !hasArg || arg == "" {
			return rule{}, fmt.Errorf("rule %q needs a pattern name", name)
		}
		re, ok := lookupPattern(arg)
		if !ok {
			return rule{}, fmt.Errorf("pattern %q is not registered", arg)
		}
		return rule{kind: rulePattern, re: re, name: arg}, nil
	case "min", "max":
		kind := ruleMin
		if name == "max" {
			kind = ruleMax
		}
		if !hasArg {
			return rule{}, fmt.Errorf("rule %q needs a value", name)
		}
		r := rule{kind: kind, arg: arg}
		switch cat {
		case catInt:
			v, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				return rule{}, fmt.Errorf("rule %q: %q is not an integer", name, arg)
			}
			r.ival = v
		case catUint:
			v, err := strconv.ParseUint(arg, 10, 64)
			if err != nil {
				return rule{}, fmt.Errorf("rule %q: %q is not an unsigned integer", name, arg)
			}
			r.uval = v
		case catFloat:
			v, err := strconv.ParseFloat(arg, 64)
			if err != nil || math.IsNaN(v) {
				return rule{}, fmt.Errorf("rule %q: %q is not a number", name, arg)
			}
			r.fval = v
		case catString, catSlice, catMap:
			v, err := strconv.ParseInt(arg, 10, 64)
			if err != nil || v < 0 {
				return rule{}, fmt.Errorf("rule %q: %q is not a length", name, arg)
			}
			r.ival = v
		default:
			return rule{}, fmt.Errorf("rule %q does not apply to %s", name, base)
		}
		return r, nil
	}
	return rule{}, fmt.Errorf("unknown rule %q", name)
}

// validEmail accepts a bare address (no display name, no angle brackets).
func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s
}

// structPlan is the analysed form of one struct type.
type structPlan struct {
	fields []fieldPlan
	byName map[string]int
}

type fieldPlan struct {
	name  string
	index int
	typ   reflect.Type
	rules []rule
}

func (f fieldPlan) optional() bool { return f.typ.Kind() == reflect.Pointer }

var plans sync.Map // reflect.Type -> *structPlan

// mustPlan returns the plan of a type that Decode has compiled. A miss panics,
// which is a programming error; the function allows panic for that reason.
//
//effectgo:allow panic
func mustPlan(t reflect.Type) *structPlan {
	p, err := planFor(t)
	if err != nil {
		// Decode compiles its type first, so a cached-plan miss here means a
		// type that Compile never saw; that is a programming error.
		panic(err)
	}
	return p
}

// planFor analyses a struct type's fields and tags, and caches the result.
// A failed analysis is not cached, so a pattern registered later is seen.
func planFor(t reflect.Type) (*structPlan, error) {
	if p, ok := plans.Load(t); ok {
		return p.(*structPlan), nil
	}
	p := &structPlan{byName: map[string]int{}}
	for i := range t.NumField() {
		f := t.Field(i)
		jsonTag := f.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		if !f.IsExported() {
			if f.Tag.Get("schema") != "" {
				return nil, fmt.Errorf("schema: %s.%s: unexported field has a schema tag", t, f.Name)
			}
			continue
		}
		if f.Anonymous {
			return nil, fmt.Errorf("schema: %s.%s: embedded fields are not supported", t, f.Name)
		}
		name, _, _ := strings.Cut(jsonTag, ",")
		if name == "" {
			name = f.Name
		}
		if _, dup := p.byName[name]; dup {
			return nil, fmt.Errorf("schema: %s: duplicate field name %q", t, name)
		}
		rules, err := parseRules(f.Tag.Get("schema"), f.Type)
		if err != nil {
			return nil, fmt.Errorf("schema: %s.%s: %w", t, f.Name, err)
		}
		p.byName[name] = len(p.fields)
		p.fields = append(p.fields, fieldPlan{name: name, index: i, typ: f.Type, rules: rules})
	}
	actual, _ := plans.LoadOrStore(t, p)
	return actual.(*structPlan), nil
}
