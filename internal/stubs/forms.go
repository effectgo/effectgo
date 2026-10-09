package stubs

// Value forms of the status, route and errdefault verbs. The verbs themselves
// are parsed in stubs.go; this file reads the richer values they accept.
//
//	status  <(pkg.Type) | pkg.Func | pkg.Var> <code | arg:N>
//	route   pkg.Func <METHOD | @N.Field> <N | @N.Field> <handlerArgIndex>
//	errdefault <pkgpath> passthrough=iface:pkg.Iface,fallback=<code>
//
// A "@N.Field" token names a field of the composite literal passed as the
// N-th argument (huma.Register(api, huma.Operation{Method: …, Path: …}, h)).

import (
	"fmt"
	"go/token"
	"strconv"
	"strings"
)

// StatusValue is the value of a status rule: a constant code, or the index of
// the argument whose constant value is the code (arg:N, for functions such as
// huma.NewError(status, …)).
type StatusValue struct {
	Code   int // the constant code; 0 when the value is an argument index
	ArgIdx int // the argument index; -1 when the value is a constant code
}

// ParseStatusValue parses a status rule's value.
func ParseStatusValue(s string) (StatusValue, error) {
	if rest, ok := strings.CutPrefix(s, "arg:"); ok {
		n, err := strconv.Atoi(rest)
		if err != nil || n < 0 {
			return StatusValue{}, fmt.Errorf("status argument index %q must be a non-negative integer", rest)
		}
		return StatusValue{ArgIdx: n}, nil
	}
	code, err := strconv.Atoi(s)
	if err != nil || code < 100 || code > 599 {
		return StatusValue{}, fmt.Errorf("status wants an HTTP status code (100-599) or arg:N, got %q", s)
	}
	return StatusValue{Code: code, ArgIdx: -1}, nil
}

// FieldRef is a "@N.Field" token: the field Field of the composite literal that
// is the N-th argument of the call.
type FieldRef struct {
	Arg   int
	Field string
}

// ParseFieldRef parses an "@N.Field" token. ok is false when s is not of that
// form (it may then be a plain index or a method); err is set when it has the
// form but is malformed.
func ParseFieldRef(s string) (ref FieldRef, ok bool, err error) {
	rest, has := strings.CutPrefix(s, "@")
	if !has {
		return FieldRef{}, false, nil
	}
	num, field, found := strings.Cut(rest, ".")
	n, convErr := strconv.Atoi(num)
	if !found || convErr != nil || n < 0 || !token.IsIdentifier(field) {
		return FieldRef{}, true, fmt.Errorf("field reference %q: want @N.Field", s)
	}
	return FieldRef{Arg: n, Field: field}, true, nil
}

// RouteArg is the path or method token of a route rule: a constant argument
// index, or a field of a composite-literal argument.
type RouteArg struct {
	Index int       // the argument index, when Field is nil
	Field *FieldRef // set for "@N.Field"
}

// ParseRouteArg parses a route rule's path token: "N" or "@N.Field".
func ParseRouteArg(s string) (RouteArg, error) {
	if ref, ok, err := ParseFieldRef(s); ok {
		if err != nil {
			return RouteArg{}, err
		}
		return RouteArg{Index: ref.Arg, Field: &ref}, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return RouteArg{}, fmt.Errorf("route argument index %q must be a non-negative integer or @N.Field", s)
	}
	return RouteArg{Index: n}, nil
}

// Errdefault is the policy of an errdefault rule: the interface whose
// implementers keep the status they carry (passthrough), and the status of
// every other error (fallback).
type Errdefault struct {
	Iface    string // qualified interface name, pkgpath.Name
	Fallback int    // status of every error that is not passed through
}

// ParseErrdefault parses "passthrough=iface:pkg.Iface,fallback=<code>".
func ParseErrdefault(s string) (Errdefault, error) {
	var out Errdefault
	seenPass, seenFallback := false, false
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return Errdefault{}, fmt.Errorf("errdefault %q: want key=value pairs", part)
		}
		switch k {
		case "passthrough":
			iface, ok := strings.CutPrefix(v, "iface:")
			if !ok {
				return Errdefault{}, fmt.Errorf("errdefault passthrough %q: want iface:pkgpath.Name", v)
			}
			if _, _, err := SplitQual(iface); err != nil {
				return Errdefault{}, fmt.Errorf("errdefault passthrough: %v", err)
			}
			out.Iface, seenPass = iface, true
		case "fallback":
			code, err := strconv.Atoi(v)
			if err != nil || code < 100 || code > 599 {
				return Errdefault{}, fmt.Errorf("errdefault fallback wants an HTTP status code (100-599), got %q", v)
			}
			out.Fallback, seenFallback = code, true
		default:
			return Errdefault{}, fmt.Errorf("errdefault: unknown key %q (want passthrough or fallback)", k)
		}
	}
	if !seenPass || !seenFallback {
		return Errdefault{}, fmt.Errorf("errdefault wants both passthrough=iface:… and fallback=<code>")
	}
	return out, nil
}
