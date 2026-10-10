// Package stubs parses the stub file format shared by the effects and errset
// analyzers. A stub file describes the effects and error variants of code that
// the analyzers do not analyze, the way a TypeScript .d.ts file describes code
// without its body:
//
//	effects <target> <effect,effect,...>   # or the word: pure; an item may be via:<argIndex>.<Method>
//	errors  <target> <variant,variant,...> # or the word: none
//	status  (pkgpath.Type) <code>           // or: pkgpath.Func / pkgpath.Var <code | arg:N>
//	route   pkgpath.Func <METHOD|@N.Field> <pathArgIndex|@N.Field> <handlerArgIndex>
//	errdefault <pkgpath> passthrough=iface:pkgpath.Iface,fallback=<code>
//	sink    <target> status=<argIndex> [body=<argIndex>]
//
// '#' starts a comment and fields are separated by whitespace. A target is
// pkgpath.Func, (pkgpath.Type).Method, (*pkgpath.Type).Method, or
// (pkgpath.Iface).Method; the predeclared error type is written (error).Error.
// Two more target forms name a function value rather than a declared function:
// pkgpath.Var (a package-level variable of func type), and (pkgpath.Type), a
// named func type such as context.CancelFunc, which applies to every call
// through a value of that type.
// A status target is a type, (pkgpath.Type). A route target is a function.
//
// This package checks the syntax that all analyzers share: the verb, the field
// count, the target and the shape of the value. Each analyzer then consumes the
// rules of its own verbs: effects reads effects rules, errset reads errors and
// status rules, openapi reads route rules. Analyzers ignore the other verbs.
//
// Which files are loaded: the -stubs flag (comma-separated paths), or, when
// the flag is empty, the EFFECTGO_STUBS environment variable.
package stubs

import (
	"fmt"
	"go/token"
	"os"
	"strconv"
	"strings"
)

// Env is the environment variable that names stub files when -stubs is empty.
const Env = "EFFECTGO_STUBS"

// Flag is the value of the -stubs flag. cmd/effectgo registers it on the
// command line; the analyzers read it through Paths.
var Flag string

// The verbs of a stub rule.
const (
	VerbEffects = "effects"
	VerbErrors  = "errors"
	VerbStatus  = "status"
	VerbRoute   = "route"
	VerbSink    = "sink"
	// VerbErrdefault is the default errmap of the routes of a package.
	VerbErrdefault = "errdefault"
)

// verbArity is the number of fields of each verb, the verb included.
var verbArity = map[string]int{
	VerbEffects:    3,
	VerbErrors:     3,
	VerbStatus:     3,
	VerbRoute:      5,
	VerbErrdefault: 3,
}

// Methods accepted by a route rule.
var routeMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// Paths returns the stub files named by Flag, or by $EFFECTGO_STUBS when Flag
// is blank.
func Paths() []string {
	src := Flag
	if strings.TrimSpace(src) == "" {
		src = os.Getenv(Env)
	}
	var out []string
	for _, p := range strings.Split(src, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Target is the parsed <target> of a rule.
type Target struct {
	Pkg    string // import path; "" for the predeclared error type
	Type   string // named type of a method
	Name   string // function or method name
	Method bool   // (pkg.T).M or (*pkg.T).M
	Ptr    bool   // (*pkg.T).M
	// TypeOnly is set for a named func type target (pkg.T): it names the
	// type, and the rule applies to calls through values of that type.
	TypeOnly bool
	// Field is set for a field source target, field:(pkg.T).F: Name is the
	// field F of the struct type T (see ParseFieldTarget).
	Field bool
}

// String renders the target in stub syntax.
func (t Target) String() string {
	if t.Field {
		return "field:(" + qualifiedType(t) + ")." + t.Name
	}
	if t.TypeOnly {
		q := t.Type
		if t.Pkg != "" {
			q = t.Pkg + "." + t.Type
		}
		return "(" + q + ")"
	}
	if !t.Method {
		return t.Pkg + "." + t.Name
	}
	star := ""
	if t.Ptr {
		star = "*"
	}
	q := t.Type
	if t.Pkg != "" {
		q = t.Pkg + "." + t.Type
	}
	return "(" + star + q + ")." + t.Name
}

// qualifiedType is the pkgpath.Type of a target, or Type alone for a type in
// no package.
func qualifiedType(t Target) string {
	if t.Pkg == "" {
		return t.Type
	}
	return t.Pkg + "." + t.Type
}

// FieldPrefix starts the target of a field source rule.
const FieldPrefix = "field:"

// ParseFieldTarget parses a field source target, field:(pkgpath.Type).Field:
// the field Field of the struct type pkgpath.Type. A field of a pointer or of
// the predeclared error type is not a field source.
func ParseFieldTarget(s string) (Target, error) {
	rest, ok := strings.CutPrefix(s, FieldPrefix)
	if !ok || !strings.HasPrefix(rest, "(") {
		return Target{}, fmt.Errorf("target %q: want field:(pkgpath.Type).Field", s)
	}
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return Target{}, fmt.Errorf("target %q: missing ')'", s)
	}
	inner := rest[1:end]
	field, ok := strings.CutPrefix(rest[end+1:], ".")
	if !ok || !token.IsIdentifier(field) {
		return Target{}, fmt.Errorf("target %q: want field:(pkgpath.Type).Field", s)
	}
	if strings.HasPrefix(inner, "*") || inner == "error" {
		return Target{}, fmt.Errorf("target %q: a field source is a field of a named struct type, not a pointer or error", s)
	}
	pkg, typ, err := SplitQual(inner)
	if err != nil {
		return Target{}, fmt.Errorf("target %v", err)
	}
	return Target{Pkg: pkg, Type: typ, Name: field, Field: true}, nil
}

// ParseTarget parses a target written as pkgpath.Func, (pkgpath.Type).Method,
// (*pkgpath.Type).Method or (error).Error.
func ParseTarget(s string) (Target, error) {
	if strings.HasPrefix(s, FieldPrefix) {
		return Target{}, fmt.Errorf("target %q: a field source (field:) is an effects rule only", s)
	}
	if !strings.HasPrefix(s, "(") {
		pkg, name, err := SplitQual(s)
		if err != nil {
			return Target{}, fmt.Errorf("target %v", err)
		}
		return Target{Pkg: pkg, Name: name}, nil
	}
	end := strings.IndexByte(s, ')')
	if end < 0 {
		return Target{}, fmt.Errorf("target %q: missing ')'", s)
	}
	rest := s[end+1:]
	if rest == "" {
		// (pkgpath.Type): a named func type. The predeclared error type is
		// not a func type, so only a qualified name is accepted.
		inner := s[1:end]
		if strings.HasPrefix(inner, "*") || inner == "error" {
			return Target{}, fmt.Errorf("target %q: want (pkgpath.Type) for a named func type", s)
		}
		pkg, typ, err := SplitQual(inner)
		if err != nil {
			return Target{}, fmt.Errorf("target %v", err)
		}
		return Target{Pkg: pkg, Type: typ, TypeOnly: true}, nil
	}
	if !strings.HasPrefix(rest, ".") || !token.IsIdentifier(rest[1:]) {
		return Target{}, fmt.Errorf("target %q: want (pkgpath.Type).Method", s)
	}
	t := Target{Name: rest[1:], Method: true}
	inner := s[1:end]
	if strings.HasPrefix(inner, "*") {
		t.Ptr = true
		inner = inner[1:]
	}
	if inner == "error" {
		t.Type = "error"
		return t, nil
	}
	pkg, typ, err := SplitQual(inner)
	if err != nil {
		return Target{}, fmt.Errorf("target %v", err)
	}
	t.Pkg, t.Type = pkg, typ
	return t, nil
}

// ParseTypeTarget parses a type target written (pkgpath.Type). The result has
// Method false and an empty Name.
func ParseTypeTarget(s string) (Target, error) {
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return Target{}, fmt.Errorf("target %q: want (pkgpath.Type)", s)
	}
	inner := s[1 : len(s)-1]
	if strings.HasPrefix(inner, "*") {
		return Target{}, fmt.Errorf("target %q: a status target is a value type, not a pointer", s)
	}
	pkg, typ, err := SplitQual(inner)
	if err != nil {
		return Target{}, fmt.Errorf("target %v", err)
	}
	return Target{Pkg: pkg, Type: typ}, nil
}

// SplitQual splits "pkgpath.Name" at its last dot.
func SplitQual(s string) (string, string, error) {
	i := strings.LastIndexByte(s, '.')
	if i <= 0 || i == len(s)-1 {
		return "", "", fmt.Errorf("%q is not a qualified name (want pkgpath.Name)", s)
	}
	pkg, name := s[:i], s[i+1:]
	if !token.IsIdentifier(name) {
		return "", "", fmt.Errorf("%q: %q is not an identifier", s, name)
	}
	return pkg, name, nil
}

// Rule is one line of a stub file.
type Rule struct {
	File   string
	Line   int
	Verb   string // one of the Verb constants
	Target Target
	Value  string   // the third field, unparsed: an effect list or "pure" (effects); a variant list or "none" (errors); the code (status); the method (route)
	Args   []string // route only: the path and handler argument indexes
}

// ParseFile reads and parses one stub file. See Parse.
func ParseFile(path string) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("stub file: %w", err)
	}
	return Parse(path, data)
}

// Load parses every file in paths, in order. Later rules override earlier
// ones; the analyzers decide what an override means.
func Load(paths []string) ([]Rule, error) {
	var all []Rule
	for _, p := range paths {
		rules, err := ParseFile(p)
		if err != nil {
			return nil, err
		}
		all = append(all, rules...)
	}
	return all, nil
}

// Parse parses the text of one stub file named name. Every error names the
// file and line.
func Parse(name string, data []byte) ([]Rule, error) {
	var rules []Rule
	for i, raw := range strings.Split(string(data), "\n") {
		line := raw
		if j := strings.IndexByte(line, '#'); j >= 0 {
			line = line[:j]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s:%d: %s", name, i+1, fmt.Sprintf(format, a...))
		}
		verb := f[0]
		if verb == VerbSink {
			r, err := parseSink(f)
			if err != nil {
				return nil, fail("%v", err)
			}
			r.File, r.Line = name, i+1
			rules = append(rules, r)
			continue
		}
		arity, known := verbArity[verb]
		if !known {
			return nil, fail("unknown verb %q (want effects, errors, status, route, errdefault or sink)", verb)
		}
		if len(f) != arity {
			return nil, fail("%s", arityMessage(verb, len(f)))
		}
		r := Rule{File: name, Line: i + 1, Verb: verb, Value: f[2]}
		var err error
		switch {
		case verb == VerbStatus && strings.HasPrefix(f[1], "("):
			r.Target, err = ParseTypeTarget(f[1])
		case verb == VerbErrdefault:
			r.Target, err = Target{Pkg: f[1]}, nil
			if strings.ContainsAny(f[1], "()") {
				err = fmt.Errorf("errdefault target %q: want a package path", f[1])
			}
		case verb == VerbEffects && strings.HasPrefix(f[1], FieldPrefix):
			r.Target, err = ParseFieldTarget(f[1])
		default:
			r.Target, err = ParseTarget(f[1])
		}
		if err != nil {
			return nil, fail("%v", err)
		}
		switch verb {
		case VerbEffects:
			_, vias, stores, err := ParseEffectsValue(f[2])
			if err != nil {
				return nil, fail("%v", err)
			}
			if r.Target.Field && (len(vias) > 0 || len(stores) > 0) {
				return nil, fail("field source %q takes fixed effects only, not via or stores items", f[1])
			}
			if len(vias) > 0 && r.Target.TypeOnly {
				return nil, fail("via items need a function or a method target, not %q", f[1])
			}
			for _, v := range vias {
				if v.Stored && !r.Target.Method {
					return nil, fail("via:stored items need a method target, not %q", f[1])
				}
			}
			if len(stores) > 0 && (r.Target.Method || r.Target.TypeOnly) {
				return nil, fail("stores items need a function target, not %q", f[1])
			}
		case VerbStatus:
			if _, err := ParseStatusValue(f[2]); err != nil {
				return nil, fail("%v", err)
			}
		case VerbRoute:
			if r.Target.Method || r.Target.TypeOnly {
				return nil, fail("route target %q must be a function, not a method or a type", f[1])
			}
			if _, ok, _ := ParseFieldRef(f[2]); !ok && !routeMethods[f[2]] {
				return nil, fail("route method %q (want GET, POST, PUT, PATCH, DELETE or @N.Field)", f[2])
			}
			if _, ok, err := ParseFieldRef(f[2]); ok && err != nil {
				return nil, fail("%v", err)
			}
			r.Value = f[2]
			r.Args = []string{f[3], f[4]}
			if _, err := ParseRouteArg(f[3]); err != nil {
				return nil, fail("%v", err)
			}
			if n, err := strconv.Atoi(f[4]); err != nil || n < 0 {
				return nil, fail("route handler index %q must be a non-negative integer", f[4])
			}
		case VerbErrdefault:
			if _, err := ParseErrdefault(f[2]); err != nil {
				return nil, fail("%v", err)
			}
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// parseSink reads "sink <target> status=<i> [body=<j>]". Args holds the two
// argument indexes: the status index, and the body index ("-1" when the rule
// has no body argument). A sink takes a variable number of fields, so it is
// read outside the arity table.
func parseSink(f []string) (Rule, error) {
	if len(f) != 3 && len(f) != 4 {
		return Rule{}, fmt.Errorf("sink wants <target> status=<argIndex> [body=<argIndex>], got %d fields", len(f))
	}
	t, err := ParseTarget(f[1])
	if err != nil {
		return Rule{}, err
	}
	if t.TypeOnly {
		return Rule{}, fmt.Errorf("sink target %q must be a function or a method", f[1])
	}
	r := Rule{Verb: VerbSink, Target: t, Args: []string{"", "-1"}}
	for _, kv := range f[2:] {
		key, val, ok := strings.Cut(kv, "=")
		n, convErr := strconv.Atoi(val)
		if !ok || convErr != nil || n < 0 {
			return Rule{}, fmt.Errorf("sink field %q: want status=<argIndex> or body=<argIndex>", kv)
		}
		switch key {
		case "status":
			r.Args[0] = val
		case "body":
			r.Args[1] = val
		default:
			return Rule{}, fmt.Errorf("sink field %q: want status=<argIndex> or body=<argIndex>", kv)
		}
	}
	if r.Args[0] == "" {
		return Rule{}, fmt.Errorf("sink wants status=<argIndex>")
	}
	return r, nil
}

func arityMessage(verb string, got int) string {
	switch verb {
	case VerbEffects:
		return fmt.Sprintf("effects wants <target> <effects> with no spaces, got %d fields", got)
	case VerbErrors:
		return fmt.Sprintf("errors wants <target> <variants> with no spaces, got %d fields", got)
	case VerbStatus:
		return fmt.Sprintf("status wants (pkgpath.Type) <code>, got %d fields", got)
	default:
		return fmt.Sprintf("route wants <target> <METHOD> <pathArgIndex> <handlerArgIndex>, got %d fields", got)
	}
}
