// Package stubs parses the stub file format shared by the effects and errset
// analyzers. A stub file describes the effects and error variants of code that
// the analyzers do not analyze, the way a TypeScript .d.ts file describes code
// without its body:
//
//	effects <target> <effect,effect,...>   # or the word: pure
//	errors  <target> <variant,variant,...> # or the word: none
//
// '#' starts a comment and fields are separated by whitespace. A target is
// pkgpath.Func, (pkgpath.Type).Method, (*pkgpath.Type).Method, or
// (pkgpath.Iface).Method; the predeclared error type is written (error).Error.
//
// This package checks the syntax that both analyzers share: the verb, the
// field count and the target. Each analyzer then checks and consumes the rules
// of its own verb: effects reads effects rules, errset reads errors rules.
// Both analyzers accept a file that mixes the two verbs.
//
// Which files are loaded: the -stubs flag (comma-separated paths), or, when
// the flag is empty, the EFFECTGO_STUBS environment variable.
package stubs

import (
	"fmt"
	"go/token"
	"os"
	"strings"
)

// Env is the environment variable that names stub files when -stubs is empty.
const Env = "EFFECTGO_STUBS"

// Flag is the value of the -stubs flag. cmd/effectgo registers it on the
// command line; the analyzers read it through Paths.
var Flag string

// The two verbs of a stub rule.
const (
	VerbEffects = "effects"
	VerbErrors  = "errors"
)

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
}

// String renders the target in stub syntax.
func (t Target) String() string {
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

// ParseTarget parses a target written as pkgpath.Func, (pkgpath.Type).Method,
// (*pkgpath.Type).Method or (error).Error.
func ParseTarget(s string) (Target, error) {
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
	Verb   string // VerbEffects or VerbErrors
	Target Target
	Value  string // the third field, unparsed: an effect list or "pure" for effects; a variant list or "none" for errors
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
		if verb != VerbEffects && verb != VerbErrors {
			return nil, fail("unknown verb %q (want effects or errors)", verb)
		}
		if len(f) != 3 {
			label := "variants"
			if verb == VerbEffects {
				label = "effects"
			}
			return nil, fail("%s wants <target> <%s> with no spaces, got %d fields", verb, label, len(f))
		}
		t, err := ParseTarget(f[1])
		if err != nil {
			return nil, fail("%v", err)
		}
		rules = append(rules, Rule{File: name, Line: i + 1, Verb: verb, Target: t, Value: f[2]})
	}
	return rules, nil
}
