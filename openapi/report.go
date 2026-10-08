package openapi

// Output of the openapi analyzer's routes as OpenAPI 3 error responses: the
// fragment (Fragment), a merge into an existing document (Merge), and a drift
// report against it (Check). Documents are generic JSON values (map[string]any),
// so that everything that is not an error response is kept as it was.

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// ExtErrors is the response extension that lists the variants of a response.
const ExtErrors = "x-effectgo-errors"

// ExtHandler is the operation extension that names the handler.
const ExtHandler = "x-effectgo-handler"

// Op is the aggregated errors of one operation (method and path). Routes of
// several packages that register the same operation are merged.
type Op struct {
	Method   string // lower case, as OpenAPI writes it
	Path     string
	Handlers []string
	Errors   []RouteError
	// Complete is true when every route of the operation has known statuses.
	Complete bool
}

// Operations groups the documented routes by path and method. Undocumented
// routes are returned separately, with their reasons, and are not in Ops.
func Operations(routes []Route) (ops []*Op, undocumented []Route) {
	byKey := map[string]*Op{}
	var order []string
	for _, r := range routes {
		if r.Undocumented != "" || r.Path == "" {
			undocumented = append(undocumented, r)
			continue
		}
		key := strings.ToLower(r.Method) + " " + r.Path
		op, ok := byKey[key]
		if !ok {
			op = &Op{Method: strings.ToLower(r.Method), Path: r.Path, Complete: true}
			byKey[key] = op
			order = append(order, key)
		}
		op.Handlers = append(op.Handlers, r.Handler)
		op.Errors = append(op.Errors, r.Errors...)
		op.Complete = op.Complete && r.Complete
	}
	for _, k := range order {
		ops = append(ops, byKey[k])
	}
	return ops, undocumented
}

// codesOf returns the explicit codes of op and whether it has a default
// (unknown status) response.
func codesOf(op *Op) (codes map[int]bool, hasDefault bool) {
	codes = map[int]bool{}
	for _, e := range op.Errors {
		if e.Status == 0 {
			hasDefault = true
			continue
		}
		codes[e.Status] = true
	}
	return codes, hasDefault
}

// responseFor builds the OpenAPI response of one status (0 for default) from
// the variants that produce it.
func responseFor(status int, errs []RouteError) map[string]any {
	var descs []string
	var list []any
	for _, e := range errs {
		d := e.Variant
		if len(e.Via) > 0 {
			d += " (via " + strings.Join(e.Via, " → ") + ")"
		}
		descs = append(descs, d)
		list = append(list, map[string]any{"variant": e.Variant, "via": strings.Join(e.Via, " → ")})
	}
	label := "Unknown status"
	if status != 0 {
		label = http.StatusText(status)
		if label == "" {
			label = "HTTP " + strconv.Itoa(status)
		}
	}
	return map[string]any{
		"description": label + ": " + strings.Join(descs, "; "),
		ExtErrors:     list,
	}
}

// responsesOf builds the error responses of an operation. The status 0 is the
// "default" key.
func responsesOf(op *Op) map[string]any {
	byStatus := map[int][]RouteError{}
	for _, e := range op.Errors {
		byStatus[e.Status] = append(byStatus[e.Status], e)
	}
	out := map[string]any{}
	for status, errs := range byStatus {
		key := "default"
		if status != 0 {
			key = strconv.Itoa(status)
		}
		out[key] = responseFor(status, errs)
	}
	return out
}

// operationOf builds the OpenAPI operation object that holds the error
// responses (the fragment's operation).
func operationOf(op *Op) map[string]any {
	return map[string]any{
		"responses": responsesOf(op),
		ExtHandler:  strings.Join(uniq(op.Handlers), ", "),
	}
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Fragment returns the OpenAPI fragment: paths.<path>.<method>.responses.
func Fragment(ops []*Op) map[string]any {
	paths := map[string]any{}
	for _, op := range ops {
		item, _ := paths[op.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[op.Path] = item
		}
		item[op.Method] = operationOf(op)
	}
	return map[string]any{"paths": paths}
}

// isErrorCode reports whether a response key of an OpenAPI document is an
// error (4xx or 5xx) or the default response.
func isErrorCode(key string) bool {
	if key == "default" {
		return true
	}
	n, err := strconv.Atoi(key)
	return err == nil && n >= 400
}

// Merge adds the error responses of ops to an OpenAPI document. For each
// operation of the document that has routes, the error responses (4xx, 5xx and
// default) are replaced by the ones the code produces; everything else is kept.
// An operation the document does not have is returned in notes. When the
// document has the HTTPError schema, the inserted responses reference it.
func Merge(doc map[string]any, ops []*Op) (merged map[string]any, notes []string) {
	paths, _ := doc["paths"].(map[string]any)
	schemaRef := httpErrorRef(doc)
	for _, op := range ops {
		item, _ := paths[op.Path].(map[string]any)
		specOp, _ := item[op.Method].(map[string]any)
		if specOp == nil {
			notes = append(notes, fmt.Sprintf("not merged: %s %s is not in the document", strings.ToUpper(op.Method), op.Path))
			continue
		}
		responses, _ := specOp["responses"].(map[string]any)
		if responses == nil {
			responses = map[string]any{}
		}
		for key := range responses {
			if isErrorCode(key) {
				delete(responses, key)
			}
		}
		for key, resp := range responsesOf(op) {
			r := resp.(map[string]any)
			if schemaRef != nil {
				r["content"] = map[string]any{"application/json": map[string]any{"schema": schemaRef}}
			}
			responses[key] = r
		}
		specOp["responses"] = responses
		specOp[ExtHandler] = strings.Join(uniq(op.Handlers), ", ")
	}
	return doc, notes
}

// httpErrorRef returns a $ref to the HTTPError component schema when the
// document has one, and nil otherwise.
func httpErrorRef(doc map[string]any) map[string]any {
	comps, _ := doc["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	if _, ok := schemas["HTTPError"]; !ok {
		return nil
	}
	return map[string]any{"$ref": "#/components/schemas/HTTPError"}
}

// Drift is one difference between the code and the document for an operation.
type Drift struct {
	Op     string // "PUT /v3/todos/{id}"
	Kind   string // "missing", "stale", "not in document", or "unchecked"
	Code   string // "404", "default", or "" for an operation-level line
	Detail string
	Blocks bool // true for a missing code, which fails the check
}

// Check compares the operations with the document's error responses. Missing
// codes (the code produces them, the document does not) are reported and block.
// Stale codes (the document documents them, the code cannot produce them) are
// reported only for a complete operation, where every status is known.
func Check(doc map[string]any, ops []*Op, undocumented []Route) []Drift {
	var out []Drift
	paths, _ := doc["paths"].(map[string]any)
	for _, op := range ops {
		name := strings.ToUpper(op.Method) + " " + op.Path
		item, _ := paths[op.Path].(map[string]any)
		specOp, _ := item[op.Method].(map[string]any)
		if specOp == nil {
			out = append(out, Drift{Op: name, Kind: "not in document", Detail: "the code has this operation; the document does not"})
			continue
		}
		responses, _ := specOp["responses"].(map[string]any)
		specCodes := map[int]bool{}
		specDefault := false
		for key := range responses {
			if key == "default" {
				specDefault = true
				continue
			}
			if n, err := strconv.Atoi(key); err == nil && n >= 400 {
				specCodes[n] = true
			}
		}
		codes, hasDefault := codesOf(op)
		for _, c := range sortedCodes(codes) {
			if !specCodes[c] {
				out = append(out, Drift{Op: name, Kind: "missing", Code: strconv.Itoa(c), Blocks: true,
					Detail: variantsWith(op, c)})
			}
		}
		if hasDefault && !specDefault {
			out = append(out, Drift{Op: name, Kind: "missing", Code: "default", Blocks: true,
				Detail: variantsWith(op, 0)})
		}
		if op.Complete {
			for _, c := range sortedKeys(specCodes) {
				if !codes[c] {
					out = append(out, Drift{Op: name, Kind: "stale", Code: strconv.Itoa(c),
						Detail: "documented, but no variant produces it"})
				}
			}
			if specDefault && !hasDefault {
				out = append(out, Drift{Op: name, Kind: "stale", Code: "default",
					Detail: "documented, but every variant has a known status"})
			}
		}
	}
	for _, r := range undocumented {
		name := strings.ToUpper(r.Method) + " " + r.Path
		if r.Path == "" {
			name = strings.ToUpper(r.Method) + " (path not constant)"
		}
		out = append(out, Drift{Op: name, Kind: "unchecked", Detail: r.Undocumented + " (" + r.Pos + ")"})
	}
	return out
}

// variantsWith names the variants of op that produce status c (0: unknown).
func variantsWith(op *Op, c int) string {
	var names []string
	for _, e := range op.Errors {
		if e.Status == c {
			names = append(names, e.Variant)
		}
	}
	return strings.Join(names, ", ")
}

func sortedCodes(m map[int]bool) []int { return sortedKeys(m) }

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
