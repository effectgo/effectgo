// Package huma is the Huma v2 adapter of the HTTP boundary check. It finds the
// routes of a package (the route rules of the stub files), resolves each route's
// handler, and maps the handler's error set through the framework's default
// errmap (the errdefault rule): an error that carries its own status keeps it,
// every other error becomes 500. The result feeds `effectgo http-huma`, which
// classifies the variants that reach 500 (see report.go).
//
// The analyzer reads only the errset analyzer's results and facts. No route
// needs an //effectgo:handler directive: Huma's handlers are literals, and the
// default errmap is a stub rule, not a source annotation.
//
// A route whose method or path is not a constant, or whose handler cannot be
// resolved to a function with an error set, is reported as unresolved with a
// reason. Nothing is guessed.
package huma

import (
	"go/ast"
	"go/constant"
	"go/types"
	"reflect"
	"sort"
	"strconv"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/effectgo/effectgo/errset"
	"github.com/effectgo/effectgo/internal/stubs"
)

// Route is one registered route of the analyzed package.
type Route struct {
	Method string // GET, POST, PUT, PATCH or DELETE; "" when unresolved
	Path   string // the constant path; "" when unresolved
	Pos    string // file:line:column of the route call
	// Handler is "func literal", or the name of the handler function or
	// constructor.
	Handler    string
	HandlerPos string // file:line:column of the handler's declaration or literal
	// Unresolved is the reason the route has no variants; "" when it is resolved.
	Unresolved string
	// Variants are the errors the handler can return, each with the status the
	// default errmap gives it. Empty when Unresolved is set.
	Variants []RouteVariant
}

// RouteVariant is one variant of a route's handler set.
type RouteVariant struct {
	Display string   // the variant's display name
	Key     string   // the variant's identity (its package and name)
	Kind    string   // errset.Kind* of the variant
	Status  int      // the HTTP status the default errmap gives it; 0 when unknown
	Known   bool     // Status is known
	Carried bool     // the error carries its own status (a status-function rule named it)
	Via     []string // witness chain from the handler down to the variant's origin
}

// Mapping is one piece of evidence that a mapper function maps a variant to a
// status: the variant's Key goes to Status in the function Mapper.
type Mapping struct {
	Key    string
	Status int
	Mapper string // the mapper's name, as errset displays it
	Pos    string // file:line of the mapper
}

// Result is the ResultType of the analyzer: the routes and the mapper evidence
// of one package.
type Result struct {
	Module   string // the analyzed module's path ("" when unknown)
	Routes   []Route
	Mappings []Mapping
}

// Analyzer is the Huma route analyzer.
var Analyzer = &analysis.Analyzer{
	Name:       "humaroutes",
	Doc:        "finds Huma routes (route stub rules), resolves their handlers, and maps their error sets through the default errmap (errdefault stub rule)",
	Requires:   []*analysis.Analyzer{errset.Analyzer},
	Run:        run,
	FactTypes:  []analysis.Fact{new(PredicateFact)},
	ResultType: reflect.TypeOf(&Result{}),
}

// routeMethods are the methods a route rule may name, and that a route may use.
var routeMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// routeRule is one "route" stub line, parsed.
type routeRule struct {
	method     string
	methodRef  *stubs.FieldRef // set when the method is a field of an argument
	path       stubs.RouteArg
	handlerArg int
}

// rules are the parsed route and errdefault rules of the stub files.
type rules struct {
	routes     map[string]routeRule        // pkgpath.Name -> rule
	errdef     map[string]stubs.Errdefault // pkgpath -> default errmap policy
	statusFunc statusFuncRules             // status-function rules (carried.go)
}

// loadRules reads the route and errdefault rules of the stub files of the
// pass's package.
func loadRules(pass *analysis.Pass) (rules, error) {
	all, err := stubs.RulesForPass(pass)
	if err != nil {
		return rules{}, err
	}
	out := rules{routes: map[string]routeRule{}, errdef: map[string]stubs.Errdefault{}, statusFunc: loadStatusFuncRules(all)}
	for _, r := range all {
		switch r.Verb {
		case stubs.VerbRoute:
			rr := routeRule{}
			if ref, ok, _ := stubs.ParseFieldRef(r.Value); ok {
				rr.methodRef = &ref
			} else {
				rr.method = r.Value
			}
			// internal/stubs has checked every token.
			rr.path, _ = stubs.ParseRouteArg(r.Args[0])
			rr.handlerArg, _ = strconv.Atoi(r.Args[1])
			out.routes[r.Target.Pkg+"."+r.Target.Name] = rr
		case stubs.VerbErrdefault:
			ed, _ := stubs.ParseErrdefault(r.Value)
			out.errdef[r.Target.Pkg] = ed
		}
	}
	return out, nil
}

func run(pass *analysis.Pass) (any, error) {
	res, _ := pass.ResultOf[errset.Analyzer].(errset.Result)
	rs, err := loadRules(pass)
	if err != nil {
		return nil, err
	}
	out := &Result{Module: moduleOf(pass)}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn := typeutil.StaticCallee(pass.TypesInfo, call)
			if fn == nil || fn.Pkg() == nil {
				return true
			}
			fn = fn.Origin()
			rule, ok := rs.routes[fn.Pkg().Path()+"."+fn.Name()]
			if !ok {
				return true
			}
			out.Routes = append(out.Routes, buildRoute(pass, res, rs, fn, rule, call))
			return true
		})
	}
	out.Mappings = mappingEvidence(pass, res, rs.statusFunc)
	sortMappings(out.Mappings)
	return out, nil
}

func moduleOf(pass *analysis.Pass) string {
	if pass.Module != nil {
		return pass.Module.Path
	}
	return ""
}

// buildRoute resolves one route call.
func buildRoute(pass *analysis.Pass, res errset.Result, rs rules, fn *types.Func, rule routeRule, call *ast.CallExpr) Route {
	r := Route{Pos: pass.Fset.Position(call.Pos()).String()}
	info := pass.TypesInfo

	method, reason := routeMethod(info, call, rule)
	if reason != "" {
		r.Unresolved = reason
		return r
	}
	r.Method = method

	path, reason := routePath(info, call, rule.path)
	if reason != "" {
		r.Unresolved = reason
		return r
	}
	r.Path = path

	if rule.handlerArg >= len(call.Args) {
		r.Unresolved = "malformed call: fewer arguments than the route rule names"
		return r
	}
	h := resolveHandler(pass, res, call.Args[rule.handlerArg])
	r.Handler, r.HandlerPos = h.name, h.pos
	if h.reason != "" {
		r.Unresolved = h.reason
		return r
	}

	ed, ok := rs.errdef[fn.Pkg().Path()]
	if !ok {
		r.Unresolved = "no errdefault rule for package " + fn.Pkg().Path() + ": the route's default errmap is unknown"
		return r
	}
	methods, ok := errset.IfaceMethods(fn.Pkg(), ed.Iface)
	if !ok {
		r.Unresolved = "errdefault interface " + ed.Iface + " is not found in the import graph"
		return r
	}
	errmap := errset.DefaultErrmap(ed.Iface, methods, ed.Fallback)

	for _, v := range errset.CollapseForeign(h.set, moduleOf(pass)) {
		code, known := errmap.StatusOf(v)
		r.Variants = append(r.Variants, RouteVariant{
			Display: v.Name, Key: v.Key, Kind: v.Kind,
			Status: code, Known: known, Carried: v.Carried,
			Via: v.Chain,
		})
	}
	return r
}

// routeMethod is the route's method: a constant, or a field of a composite
// literal argument (huma.Register's operation).
func routeMethod(info *types.Info, call *ast.CallExpr, rule routeRule) (string, string) {
	m := rule.method
	if rule.methodRef != nil {
		v, reason := fieldString(info, call, *rule.methodRef)
		if reason != "" {
			return "", "method: " + reason
		}
		m = v
	}
	if !routeMethods[m] {
		return "", "method " + quote(m) + " is not a route method (GET, POST, PUT, PATCH or DELETE)"
	}
	return m, ""
}

// routePath is the route's constant path, from a constant argument or a field.
func routePath(info *types.Info, call *ast.CallExpr, arg stubs.RouteArg) (string, string) {
	if arg.Field != nil {
		v, reason := fieldString(info, call, *arg.Field)
		if reason != "" {
			return "", "path: " + reason
		}
		return v, ""
	}
	if arg.Index >= len(call.Args) {
		return "", "malformed call: fewer arguments than the route rule names"
	}
	if s, ok := constString(info, call.Args[arg.Index]); ok {
		return s, ""
	}
	return "", "the path is not a constant string"
}

// fieldString returns the constant string field of the composite literal that
// is the ref.Arg-th argument of call. The reason is set when it cannot be read.
func fieldString(info *types.Info, call *ast.CallExpr, ref stubs.FieldRef) (string, string) {
	if ref.Arg >= len(call.Args) {
		return "", "malformed call: fewer arguments than the route rule names"
	}
	e := unparenExpr(call.Args[ref.Arg])
	if u, ok := e.(*ast.UnaryExpr); ok {
		e = unparenExpr(u.X)
	}
	lit, ok := e.(*ast.CompositeLit)
	if !ok {
		return "", "argument " + strconv.Itoa(ref.Arg) + " is not a composite literal (the value is built elsewhere), so field " + ref.Field + " is unknown"
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != ref.Field {
			continue
		}
		if s, ok := constString(info, kv.Value); ok {
			return s, ""
		}
		return "", "field " + ref.Field + " is not a constant string"
	}
	return "", "field " + ref.Field + " is not set in the literal"
}

// constString returns the value of a constant string expression.
func constString(info *types.Info, e ast.Expr) (string, bool) {
	tv, ok := info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// handler is a resolved handler argument: its error set, or the reason it has none.
type handler struct {
	name   string
	pos    string
	set    []errset.Variant
	reason string
}

// resolveHandler follows the handler argument of a route to an error set. A
// func literal is followed to its returns; a declared function to its set (its
// own, or its ErrFact when it is in another package); a call to a constructor
// to the handler set it returns. Anything else has a reason.
func resolveHandler(pass *analysis.Pass, res errset.Result, e ast.Expr) handler {
	info := pass.TypesInfo
	e = unparenExpr(e)
	switch x := e.(type) {
	case *ast.FuncLit:
		h := handler{name: "func literal", pos: pass.Fset.Position(x.Pos()).String()}
		fe, ok := res.Literals[x]
		if !ok {
			h.reason = "the handler literal does not return an error"
			return h
		}
		h.set = fe.Variants
		return h

	case *ast.CallExpr:
		fn := typeutil.StaticCallee(info, x)
		if fn == nil {
			return handler{name: "call", reason: "the handler is a call that is not to a static function"}
		}
		fn = fn.Origin()
		h := handler{name: fn.Name() + "(...)", pos: pass.Fset.Position(fn.Pos()).String()}
		set, handlerSet, ok := declaredErrors(pass, res, fn)
		switch {
		case !ok:
			h.reason = fn.Name() + " has no error set (not a handler constructor)"
		case !handlerSet:
			h.reason = fn.Name() + " does not return a func literal, so its handler is not visible"
		default:
			h.set = set
		}
		return h

	case *ast.Ident, *ast.SelectorExpr:
		var obj types.Object
		if id, ok := x.(*ast.Ident); ok {
			obj = info.Uses[id]
		} else {
			obj = info.Uses[x.(*ast.SelectorExpr).Sel]
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			return handler{name: exprText(x), reason: exprText(x) + " is a value, not a declared function"}
		}
		fn = fn.Origin()
		h := handler{name: fn.Name(), pos: pass.Fset.Position(fn.Pos()).String()}
		set, handlerSet, ok := declaredErrors(pass, res, fn)
		switch {
		case !ok:
			h.reason = fn.Name() + " has no error set (no error result)"
		case handlerSet:
			h.reason = fn.Name() + " is a handler constructor: pass the result of calling it"
		default:
			h.set = set
		}
		return h
	}
	return handler{name: "expression", reason: "the handler is not a function, a call or a func literal"}
}

// declaredErrors returns the effective error set of a declared function: its
// own set, or its handler set when it returns func literals (handlerSet). ok is
// false when the function has no error set. A function of another package is
// read from its errset fact.
func declaredErrors(pass *analysis.Pass, res errset.Result, fn *types.Func) (set []errset.Variant, handlerSet bool, ok bool) {
	if fn.Pkg() == pass.Pkg {
		fe, ok := res.Funcs[fn]
		if !ok {
			return nil, false, false
		}
		return fe.Variants, fe.Handler, true
	}
	var f errset.ErrFact
	if !pass.ImportObjectFact(fn, &f) {
		return nil, false, false
	}
	if f.HasHandler {
		return f.Handler, true, true
	}
	return f.Errors, false, true
}

func unparenExpr(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func exprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprText(x.X) + "." + x.Sel.Name
	}
	return "expression"
}

func quote(s string) string { return "\"" + s + "\"" }

// sortMappings orders the evidence by key, then mapper and status, so the output
// does not depend on map iteration order.
func sortMappings(ms []Mapping) {
	sort.SliceStable(ms, func(i, j int) bool {
		a, b := ms[i], ms[j]
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.Mapper != b.Mapper {
			return a.Mapper < b.Mapper
		}
		return a.Status < b.Status
	})
}
