// Package tsgen finds the HTTP routes of rt/web (the route rules of the stub
// files, shared with errset) and describes, for each one, its method, path,
// handler, input and output types, and the error variants that carry a constant
// status. The result feeds `effectgo ts`, which writes the TypeScript client.
//
// Facts: errset's facts are not shared with this analyzer, so tsgen exports its
// own. Every function with an error set gets a HandlerFact (its handler set, so
// a route in one package can read a handler of a dependency), and every
// StatusCode method with a constant result gets a StatusFact (the framework's own
// error types, DecodeError, BodyTooLarge and InternalError, need their status).
package tsgen

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"

	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/rtshape"
	"github.com/faintghost/effectgo/internal/stubs"
)

const (
	webPath    = "github.com/faintghost/effectgo/rt/web"
	schemaPath = "github.com/faintghost/effectgo/rt/schema"
)

// HandlerFact is exported on every function of a package that has an error set.
type HandlerFact struct {
	Set     []errset.Variant // the handler set when Handler, else the function's own set
	Handler bool
}

// AFact marks HandlerFact as an analysis fact.
func (*HandlerFact) AFact() {}

func (f *HandlerFact) String() string { return "tsgen-handler" }

// StatusFact is exported on a StatusCode method whose result is a constant.
type StatusFact struct{ Code int }

// AFact marks StatusFact as an analysis fact.
func (*StatusFact) AFact() {}

func (f *StatusFact) String() string { return "tsgen-status" }

// Analyzer is the tsgen analyzer.
var Analyzer = &analysis.Analyzer{
	Name:       "tsgen",
	Doc:        "finds the rt/web routes and describes their input, output and error types for the TypeScript generator",
	Requires:   []*analysis.Analyzer{errset.Analyzer},
	Run:        run,
	FactTypes:  []analysis.Fact{new(HandlerFact), new(StatusFact)},
	ResultType: reflect.TypeOf(Result{}),
}

// Result is the ResultType of one package.
type Result struct {
	Routes []Route
	Decls  []Decl // every named struct the routes reach, once each
}

// Route is one registered route. Problems are the reasons the generator cannot
// write the route; each is a message with its position.
type Route struct {
	Method          string
	Pattern         string // the Go pattern, e.g. /api/contacts/{id}
	Group           string // lower camel case: the receiver type, or the package name
	Name            string // lower camel case: the handler name
	Pos             string
	In              *rtshape.Shape // nil for web.NoInput
	Out             *rtshape.Shape // nil for web.NoContent
	NoContent       bool
	Binary          bool // web.FileResponse or web.StreamResponse, not a JSON schema
	File            bool // web.FileResponse, served with range support at status 200
	Redirect        bool // web.RedirectResponse has an HTML body and a Location header
	SuccessStatus   int
	SuccessStatuses []int
	Response        bool           // web.Response[T] carries body T and dynamic status metadata
	ProxyFailure    *rtshape.Shape // web.ProxyResponse[S, F] has an independent failure schema, not a fixed error status
	JSONBody        bool
	JSONBodyEmpty   bool
	Errors          []ErrRef // the error list: handled variants by status and name, then the framework's
	Problems        []string
}

// ErrRef is one entry of an endpoint's error list.
type ErrRef struct {
	Key      string // pkgpath.Name
	Name     string // the Go type name, the TypeScript class name
	Status   int
	Untagged bool // external middleware body has no required wire _tag
}

// Decl is one named struct type that a route reaches.
type Decl struct {
	Key  string // pkgpath.Name
	Name string
	Pos  string
	Type *rtshape.Type
}

// state is the state of one package.
type state struct {
	pass  *analysis.Pass
	res   errset.Result
	out   Result
	table map[string]errset.RouteRule
	decls map[string]bool
}

func run(pass *analysis.Pass) (any, error) {
	res, _ := pass.ResultOf[errset.Analyzer].(errset.Result)
	for obj, fe := range res.Funcs {
		pass.ExportObjectFact(obj, &HandlerFact{Set: fe.Variants, Handler: fe.Handler})
	}
	for fn, code := range res.Statuses {
		pass.ExportObjectFact(fn, &StatusFact{Code: code})
	}
	rules, err := stubs.RulesForPass(pass)
	if err != nil {
		return nil, err
	}
	st := &state{pass: pass, res: res, table: errset.RouteRules(rules), decls: map[string]bool{}}
	// Other HTTP frameworks share route stubs for diagnostics, but do not use
	// rt/web's handler shape, encoding, or route options.
	for target := range st.table {
		if !strings.HasPrefix(target, webPath+".") {
			delete(st.table, target)
		}
	}
	if len(st.table) == 0 {
		return Result{}, nil
	}
	for _, f := range pass.Files {
		if strings.HasSuffix(pass.Fset.Position(f.Pos()).Filename, "_test.go") {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if rule, ok := errset.RouteCallRule(pass.TypesInfo, call, st.table); ok {
					st.route(call, rule)
				}
			}
			return true
		})
	}
	return st.out, nil
}

// route describes one route call.
func (st *state) route(call *ast.CallExpr, rule errset.RouteRule) {
	info := st.pass.TypesInfo
	r := Route{Method: rule.Method, Pos: st.pos(call.Pos())}
	defer func() { st.out.Routes = append(st.out.Routes, r) }()
	if rule.PathArg >= len(call.Args) {
		r.Problems = append(r.Problems, r.Pos+": the route call has no path argument")
		return
	}
	tv := info.Types[call.Args[rule.PathArg]]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		r.Problems = append(r.Problems, r.Pos+": the path is not a constant string")
		return
	}
	r.Pattern = constant.StringVal(tv.Value)
	handler := call.Args[rule.HandlerArg]
	fn, ok := errset.HandlerFunc(info, handler)
	sig, _ := info.TypeOf(handler).(*types.Signature)
	if !ok || sig == nil || sig.Params().Len() != 2 || sig.Results().Len() != 2 {
		r.Problems = append(r.Problems, r.Pos+": the handler must be a named function or a method value of the form func(context.Context, In) (Out, error); a func literal or a call is not supported")
		return
	}
	r.Name = lowerCamel(fn.Name())
	r.Group = lowerCamel(groupName(fn))

	in, out := types.Unalias(sig.Params().At(1).Type()), types.Unalias(sig.Results().At(0).Type())
	noInput := isNamed(in, webPath, "NoInput")
	r.NoContent = isNamed(out, webPath, "NoContent")
	r.File = isNamed(out, webPath, "FileResponse")
	r.Binary = r.File || isNamed(out, webPath, "StreamResponse")
	r.Redirect = isNamed(out, webPath, "RedirectResponse")
	r.SuccessStatus = 200
	if r.NoContent {
		r.SuccessStatus = 204
	} else if r.Redirect {
		r.SuccessStatus = 302
	}
	if isNamed(out, webPath, "Response") || isNamed(out, webPath, "ProxyResponse") {
		args := out.(*types.Named).TypeArgs()
		if isNamed(out, webPath, "ProxyResponse") {
			r.ProxyFailure = st.describe(args.At(1), call.Pos(), &r)
		} else {
			r.Response = true
		}
		out = args.At(0)
	}
	r.Errors = st.errors(fn, &r, !noInput)
	st.routeOptions(call, &r)
	if !noInput {
		r.In = rtshape.Describe(in, rtshape.Options{Qualifier: st.qualifier(), Multipart: true, JSONBody: r.JSONBody})
		if r.JSONBody {
			validation := *r.In
			validation.Types = nil // compatibility bodies are inlined, never strict declarations
			st.record(&validation, call.Pos(), &r)
		} else {
			st.record(r.In, call.Pos(), &r)
		}
	}
	if !r.NoContent && !r.Redirect && !r.Binary {
		r.Out = st.describe(out, call.Pos(), &r)
	}
	st.checkPath(&r, noInput)
}

// checkPath checks the input against the pattern and method, as rt/web does at
// registration: every wildcard has a path field, every path field has a
// wildcard, and body fields need a method that reads a body.
func (st *state) checkPath(r *Route, noInput bool) {
	wild := map[string]bool{}
	for _, seg := range strings.Split(r.Pattern, "/") {
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") {
			continue
		}
		name := seg[1 : len(seg)-1]
		if strings.HasSuffix(name, "...") || name == "$" {
			r.Problems = append(r.Problems, r.Pos+": pattern "+r.Pattern+" has a wildcard that is not a path parameter; the generator writes only {name}")
			continue
		}
		wild[name] = true
	}
	if noInput {
		for name := range wild {
			r.Problems = append(r.Problems, r.Pos+": pattern "+r.Pattern+" has {"+name+"}, but the input is web.NoInput")
		}
		return
	}
	paths := map[string]bool{}
	forms := map[string]bool{}
	hasJSON := false
	for _, f := range r.In.Root.Fields {
		if f.Form != "" {
			if forms[f.Form] {
				r.Problems = append(r.Problems, r.Pos+": multipart form name "+f.Form+" is repeated")
			}
			forms[f.Form] = true
		} else if f.Path == "" && f.Query == "" && f.Header == "" && f.Cookie == "" {
			hasJSON = true
		}
	}
	if len(forms) > 0 && hasJSON {
		r.Problems = append(r.Problems, r.Pos+": multipart file fields cannot be mixed with JSON body fields")
	}
	for _, f := range r.In.Root.Fields {
		if f.Path != "" {
			paths[f.Path] = true
			if !wild[f.Path] {
				r.Problems = append(r.Problems, r.Pos+": path field "+f.Name+" has no {"+f.Path+"} in "+r.Pattern)
			}
		}
		if f.Path == "" && f.Query == "" && f.Header == "" && f.Cookie == "" && r.Method != "POST" && r.Method != "PUT" && r.Method != "PATCH" && r.Method != "DELETE" {
			r.Problems = append(r.Problems, r.Pos+": "+r.Method+" takes no body, but field "+f.Name+" has no input location tag")
		}
	}
	for name := range wild {
		if !paths[name] {
			r.Problems = append(r.Problems, r.Pos+": pattern "+r.Pattern+" has {"+name+"} with no path field")
		}
	}
}

// errors builds the error list of a route: the handler's variants with a constant
// status, sorted by status and name; then DecodeError when the route has an input,
// BodyTooLarge when it reads a body, and InternalError always (rt/web).
func (st *state) errors(fn *types.Func, r *Route, hasInput bool) []ErrRef {
	var handled []ErrRef
	seen := map[string]bool{}
	for _, v := range st.variants(fn) {
		status, ok := errset.HandledStatus(v)
		if !ok || seen[v.Key] {
			continue
		}
		seen[v.Key] = true
		named := st.lookup(v.Key)
		if named == nil {
			r.Problems = append(r.Problems, r.Pos+": cannot find the error type "+v.Key)
			continue
		}
		st.describeNamed(named, r)
		handled = append(handled, ErrRef{Key: v.Key, Name: named.Obj().Name(), Status: status})
	}
	sort.SliceStable(handled, func(i, j int) bool {
		if handled[i].Status != handled[j].Status {
			return handled[i].Status < handled[j].Status
		}
		return handled[i].Name < handled[j].Name
	})
	out := handled
	framework := []struct{ path, name string }{}
	if hasInput {
		framework = append(framework, struct{ path, name string }{schemaPath, "DecodeError"})
	}
	if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" || r.Method == "DELETE" {
		framework = append(framework, struct{ path, name string }{webPath, "BodyTooLarge"})
	}
	framework = append(framework, struct{ path, name string }{webPath, "InternalError"})
	for _, f := range framework {
		if seen[f.path+"."+f.name] {
			continue // the handler already returns it: listed once
		}
		seen[f.path+"."+f.name] = true
		named := st.lookup(f.path + "." + f.name)
		if named == nil {
			r.Problems = append(r.Problems, r.Pos+": the framework error "+f.name+" is not in the import closure")
			continue
		}
		st.describeNamed(named, r)
		out = append(out, ErrRef{Key: f.path + "." + f.name, Name: f.name, Status: st.statusOf(named, r)})
	}
	return out
}

// variants returns the error set of a handler: its own, or the handler set of the
// function, from this package's result or from the fact of a dependency.
func (st *state) variants(fn *types.Func) []errset.Variant {
	fn = fn.Origin()
	if fe, ok := st.res.Funcs[fn]; ok && fn.Pkg() == st.pass.Pkg {
		return fe.Variants
	}
	var f HandlerFact
	if st.pass.ImportObjectFact(fn, &f) {
		return f.Set
	}
	return nil
}

// statusOf is the constant status of the StatusCode method of n: from this
// package's result, or from the StatusFact of a dependency. A missing status is a
// problem, and 500 is returned so the list stays complete.
func (st *state) statusOf(n *types.Named, r *Route) int {
	obj, _, _ := types.LookupFieldOrMethod(n, true, nil, "StatusCode")
	fn, ok := obj.(*types.Func)
	if ok {
		fn = fn.Origin()
		if fn.Pkg() == st.pass.Pkg {
			if code, ok := st.res.Statuses[fn]; ok {
				return code
			}
		} else {
			var f StatusFact
			if st.pass.ImportObjectFact(fn, &f) {
				return f.Code
			}
		}
	}
	r.Problems = append(r.Problems, r.Pos+": "+n.Obj().Name()+" has no constant StatusCode")
	return 500
}

// describe describes a type of a route and records its named structs.
func (st *state) describe(t types.Type, pos token.Pos, r *Route) *rtshape.Shape {
	shape := rtshape.Describe(t, rtshape.Options{Qualifier: st.qualifier(), JSONOutput: true})
	st.record(shape, pos, r)
	return shape
}

// describeNamed describes a named error type and records its structs.
func (st *state) describeNamed(n *types.Named, r *Route) {
	st.record(rtshape.Describe(n, rtshape.Options{Qualifier: st.qualifier(), JSONOutput: true}), n.Obj().Pos(), r)
}

// recordProblems adds positioned shape diagnostics without declaring schemas.
func (st *state) recordProblems(shape *rtshape.Shape, pos token.Pos, r *Route) {
	for _, p := range shape.Problems {
		at := pos
		if p.Owner != nil {
			at = p.Owner.Pos()
		}
		msg := p.Msg
		if p.Where != "" {
			msg = p.Where + ": " + msg
		}
		r.Problems = append(r.Problems, st.pos(at)+": "+msg)
	}
}

// record turns shape problems into messages and adds named struct declarations.
func (st *state) record(shape *rtshape.Shape, pos token.Pos, r *Route) {
	st.recordProblems(shape, pos, r)
	for _, t := range shape.Types {
		if t.Cat == rtshape.CatStruct && t.Name == "" && !shape.JSONOutput {
			r.Problems = append(r.Problems, st.pos(pos)+": an anonymous struct cannot be a TypeScript schema; name it")
		}
		if t.Cat != rtshape.CatStruct || t.Name == "" || st.decls[t.Key] {
			continue
		}
		st.decls[t.Key] = true
		key := t.Key
		// A multipart endpoint uses its own form fields; named declarations
		// must retain the JSON shape when the type is reused in another body.
		for _, f := range t.Fields {
			if f.Form != "" {
				t = rtshape.Describe(t.Go, rtshape.Options{Qualifier: st.qualifier()}).Root
				break
			}
		}
		st.out.Decls = append(st.out.Decls, Decl{Key: key, Name: t.Name, Pos: st.pos(t.Go.(*types.Named).Obj().Pos()), Type: t})
	}
}

// lookup returns the named type of key pkgpath.Name, found in the import closure
// of the package, or nil.
func (st *state) lookup(key string) *types.Named {
	i := strings.LastIndex(key, ".")
	if i < 0 {
		return nil
	}
	pkg := findPkg(st.pass.Pkg, key[:i], map[*types.Package]bool{})
	if pkg == nil {
		return nil
	}
	tn, ok := pkg.Scope().Lookup(key[i+1:]).(*types.TypeName)
	if !ok {
		return nil
	}
	n, _ := tn.Type().(*types.Named)
	return n
}

// findPkg searches the import closure of p for the package with path.
func findPkg(p *types.Package, path string, seen map[*types.Package]bool) *types.Package {
	if p.Path() == path {
		return p
	}
	if seen[p] {
		return nil
	}
	seen[p] = true
	for _, imp := range p.Imports() {
		if q := findPkg(imp, path, seen); q != nil {
			return q
		}
	}
	return nil
}

func (st *state) pos(p token.Pos) string { return st.pass.Fset.Position(p).String() }

// qualifier names a type in messages: unqualified in this package.
func (st *state) qualifier() types.Qualifier {
	return func(p *types.Package) string {
		if p == st.pass.Pkg {
			return ""
		}
		return p.Name()
	}
}

// groupName is the receiver type of a method, or the package name of a function.
func groupName(fn *types.Func) string {
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		t := sig.Recv().Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		if n, ok := t.(*types.Named); ok {
			return n.Obj().Name()
		}
	}
	return fn.Pkg().Name()
}

// lowerCamel lowers the leading capitals of a Go name, as Go writes an acronym in
// a lower-case name: the run of capitals at the start is lowered, except that
// when a lower-case letter follows it, its last capital is kept, since that one
// starts the next word. API is api, HTTPServer is httpServer, URLMap is urlMap,
// Store is store, ID is id. A run of one capital is lowered alone. A plural
// acronym lowers with its s when the s ends the word or a capital or digit follows
// it: IDs is ids, IDsByName is idsByName, URLsV2 is urlsV2.
func lowerCamel(s string) string {
	rs := []rune(s)
	run := 0
	for run < len(rs) && unicode.IsUpper(rs[run]) {
		run++
	}
	if run == 0 {
		return s
	}
	// An acronym in the plural (IDs, URLs) lowers with its s, when the s ends the
	// word or a capital or digit follows it: IDs is ids, IDsByName is idsByName,
	// URLsV2 is urlsV2.
	if run < len(rs) && rs[run] == 's' && (run+1 == len(rs) || unicode.IsUpper(rs[run+1]) || unicode.IsDigit(rs[run+1])) {
		for i := 0; i <= run; i++ {
			rs[i] = unicode.ToLower(rs[i])
		}
		return string(rs)
	}
	lower := run
	if run >= 2 && run < len(rs) && unicode.IsLower(rs[run]) {
		lower = run - 1
	}
	for i := 0; i < lower; i++ {
		rs[i] = unicode.ToLower(rs[i])
	}
	return string(rs)
}

// isNamed reports whether t is the named type pkgPath.name.
func isNamed(t types.Type, pkgPath, name string) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == pkgPath && n.Obj().Name() == name
}
