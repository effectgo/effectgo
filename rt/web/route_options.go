package web

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/faintghost/effectgo/rt/schema"
)

// RouteOption declares additional HTTP boundary contracts.
// Its zero value is invalid.
type RouteOption struct {
	validate           func() error
	status             int
	statuses           []int
	jsonBody           bool
	jsonBodyEmpty      bool
	jsonBodyBestEffort bool
	jsonBodyExact      bool
	formBody           bool
}

// SuccessStatus declares the HTTP status written by a successful typed handler.
// Use a direct call with a constant code on the route so generated clients read it.
// Registration checks that the status matches the handler's response type.
func SuccessStatus(code int) RouteOption {
	return RouteOption{status: code, validate: func() error {
		if code >= 200 && code <= 299 || code == 301 || code == 302 || code == 303 || code == 307 || code == 308 {
			return nil
		}
		return fmt.Errorf("invalid success status %d: expected 2xx or a redirect status", code)
	}}
}

// SuccessStatuses declares all permitted successful response statuses for a
// web.Response, FileResponse, or StreamResponse whose status varies at runtime.
func SuccessStatuses(codes ...int) RouteOption {
	codes = slices.Clone(codes)
	return RouteOption{statuses: codes, validate: func() error {
		if len(codes) == 0 {
			return fmt.Errorf("SuccessStatuses requires at least one status")
		}
		seen := make(map[int]bool, len(codes))
		for _, code := range codes {
			if code < 200 || code > 299 || seen[code] {
				return fmt.Errorf("invalid or repeated success status %d", code)
			}
			seen[code] = true
		}
		return nil
	}}
}

// SuccessStatusRange declares every 2xx status in the inclusive range.
func SuccessStatusRange(first, last int) RouteOption {
	if first < 200 || last > 299 || first > last {
		return RouteOption{validate: func() error { return fmt.Errorf("invalid success status range %d..%d", first, last) }}
	}
	codes := make([]int, last-first+1)
	for i := range codes {
		codes[i] = first + i
	}
	return SuccessStatuses(codes...)
}

// JSONBody opts this route into encoding/json request decoding, including
// omitted fields, null, custom unmarshaling, and unknown object properties.
// URL fields are still decoded independently and take precedence over JSON.
func JSONBody() RouteOption {
	return RouteOption{jsonBody: true, validate: func() error { return nil }}
}

// JSONBodyExact requires exactly one JSON value, as encoding/json.Unmarshal does.
func JSONBodyExact() RouteOption {
	return RouteOption{jsonBody: true, jsonBodyExact: true, validate: func() error { return nil }}
}

// JSONBodyEmpty adds acceptance of an empty body to JSONBody decoding.
func JSONBodyEmpty() RouteOption {
	return RouteOption{jsonBody: true, jsonBodyEmpty: true, validate: func() error { return nil }}
}

// JSONBodyBestEffort preserves legacy handlers that ignore JSON decode errors.
// URL-owned fields are still validated before invoking the typed handler.
func JSONBodyBestEffort() RouteOption {
	return RouteOption{jsonBody: true, jsonBodyEmpty: true, jsonBodyBestEffort: true, validate: func() error { return nil }}
}

// FormBody preserves legacy multipart extraction: unknown parts are discarded
// and the first value for a declared scalar or file is used. Limits still apply.
func FormBody() RouteOption {
	return RouteOption{formBody: true, validate: func() error { return nil }}
}

// ExternalError declares an error response written outside the typed handler,
// such as an existing authentication or module middleware, or written by a custom
// WithErrorEncoder. E describes the exact JSON body and has a constant StatusCode
// method. It does not require a wire _tag: the generated client adds a
// discriminant to the decoded error instance. A declaration for an inferred
// handler error overrides that error's default tagged wire contract.
// Register these calls directly on each route so the generator can read E.
// The declaration must also pass schema.Compile at startup: embedded fields and
// invalid schema tags are rejected by registration and generation. Response
// schemas still follow encoding/json and do not apply input validation rules.
// It cannot prove middleware actually writes that body; check that integration.
func ExternalError[E interface {
	error
	StatusCode() int
}]() RouteOption {
	return RouteOption{validate: func() error {
		t := reflect.TypeFor[E]()
		if t.Kind() != reflect.Struct {
			return fmt.Errorf("%v must be a struct error with a value StatusCode method", t)
		}
		var e E
		if status := e.StatusCode(); status < 400 || status > 599 {
			return fmt.Errorf("%v has invalid error status %d", t, status)
		}
		return schema.Compile[E]()
	}}
}
