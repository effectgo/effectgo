// Package openapi finds the HTTP routes of a package and states, for each one,
// which errors its handler can return and with which HTTP status. It reads the
// route rules of the stub files (see internal/stubs), resolves each route's
// handler, and maps the handler's error set through the handler's errmap (the
// //effectgo:handler directive). The result feeds `effectgo openapi`.
//
// A route is undocumented, with a reason, when its handler cannot be resolved
// to a function with a visible error set and an errmap. Nothing is guessed: a
// variant whose status is unknown is reported with Status 0 (the OpenAPI
// "default" response).
//
// Facts: the errset analyzer's facts are not shared with this analyzer, so this
// analyzer exports its own. Every function with an error set gets a HandlerFact
// (its set, its errmap, whether it is a handler constructor), and every errmap
// function gets an ErrmapInfo. A route in one package can then resolve a handler
// declared in a dependency.
package openapi

import (
	"go/ast"
	"go/constant"
	"go/types"
	"reflect"
	"strconv"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
)

// Route is one registered HTTP route of the analyzed package.
type Route struct {
	Method string // GET, POST, PUT, PATCH or DELETE
	Path   string // the constant path argument; "" when it is not constant
	// Handler names the handler: the function (or constructor) that was passed or
	// called, or "func literal" for a literal.
	Handler string
	Pos     string // file:line:column of the route call
	// Errmap is the errmap the handler directive names, as written ("" if none).
	Errmap string
	// Undocumented is the reason the route has no error docs. "" when documented.
	Undocumented string
	// Complete is true when every variant of the handler's set has a known
	// status. Only a complete route can be checked for stale documentation.
	Complete bool
	// Errors are the variants of the handler's set, each with its status.
	Errors []RouteError
}

// RouteError is one variant that a route can return.
type RouteError struct {
	Status  int      // the HTTP status; 0 when unknown (the "default" response)
	Variant string   // display name, e.g. "domain.ErrNotFound"
	Via     []string // witness chain from the handler down to the origin of the variant
	// Opaque is true for an unclassified error: the "?" variant, or a fresh
	// "new@" variant, which no errors.Is or errors.As can match. Its status is
	// 500 unless the errmap maps it, see errset.ownStatus.
	Opaque bool
}

// Result is the ResultType: the routes of the package, in source order.
type Result []Route

// HandlerFact is exported on every function of a package that has an error set.
type HandlerFact struct {
	// Set is the effective error set: the handler set when Handler is true,
	// otherwise the function's own set.
	Set []errset.Variant
	// Handler is true when the function returns handler literals (a constructor).
	Handler bool
	// Errmap is the errmap that the function's //effectgo:handler directive
	// names, or nil.
	Errmap *errset.ErrmapFact
	// ErrmapName is the directive argument as written, "" when there is none.
	ErrmapName string
}

// AFact marks HandlerFact as an analysis fact.
func (*HandlerFact) AFact() {}

func (f *HandlerFact) String() string { return "openapi-handler" }

// ErrmapInfo is exported on every valid errmap function of a package.
type ErrmapInfo struct {
	Fact errset.ErrmapFact
}

// AFact marks ErrmapInfo as an analysis fact.
func (*ErrmapInfo) AFact() {}

func (f *ErrmapInfo) String() string { return "openapi-errmap" }

// Analyzer is the openapi analyzer.
var Analyzer = &analysis.Analyzer{
	Name:       "openapi",
	Doc:        "finds HTTP routes (route stub rules) and the errors each handler can return, with their HTTP status through the handler's errmap",
	Requires:   []*analysis.Analyzer{errset.Analyzer},
	Run:        run,
	FactTypes:  []analysis.Fact{new(HandlerFact), new(ErrmapInfo)},
	ResultType: reflect.TypeOf(Result{}),
}

// routeRule is one "route" stub line.
type routeRule struct {
	method     string
	pathArg    int
	handlerArg int
}

// routeTable reads the route rules of the stub files, keyed by pkgpath.Name.
func routeTable() (map[string]routeRule, error) {
	rules, err := stubs.Load(stubs.Paths())
	if err != nil {
		return nil, err
	}
	out := map[string]routeRule{}
	for _, r := range rules {
		if r.Verb != stubs.VerbRoute {
			continue
		}
		// internal/stubs has checked that both indexes are non-negative integers.
		p, _ := strconv.Atoi(r.Args[0])
		h, _ := strconv.Atoi(r.Args[1])
		out[r.Target.Pkg+"."+r.Target.Name] = routeRule{method: r.Value, pathArg: p, handlerArg: h}
	}
	return out, nil
}

// pass state
type state struct {
	pass     *analysis.Pass
	res      errset.Result
	handlers map[*types.Func]*HandlerFact // this package's functions
}

func run(pass *analysis.Pass) (any, error) {
	res, _ := pass.ResultOf[errset.Analyzer].(errset.Result)
	st := &state{pass: pass, res: res, handlers: map[*types.Func]*HandlerFact{}}

	for obj, fe := range res.Funcs {
		hf := &HandlerFact{Set: fe.Variants, Handler: fe.Handler, ErrmapName: fe.HandlerDirective}
		if fe.Errmap != nil {
			hf.Errmap = st.errmapOf(fe.Errmap)
		}
		st.handlers[obj] = hf
		pass.ExportObjectFact(obj, hf)
	}
	for obj, m := range res.Errmaps {
		pass.ExportObjectFact(obj, &ErrmapInfo{Fact: *m})
	}

	table, err := routeTable()
	if err != nil {
		return nil, err
	}
	var out Result
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
			rule, ok := table[fn.Origin().Pkg().Path()+"."+fn.Origin().Name()]
			if !ok {
				return true
			}
			out = append(out, st.route(call, rule))
			return true
		})
	}
	return out, nil
}

// errmapOf returns the errmap fact of an errmap function, from this package or
// from the ErrmapInfo fact of a dependency.
func (st *state) errmapOf(fn *types.Func) *errset.ErrmapFact {
	if fn.Pkg() == st.pass.Pkg {
		return st.res.Errmaps[fn]
	}
	var info ErrmapInfo
	if st.pass.ImportObjectFact(fn, &info) {
		return &info.Fact
	}
	return nil
}

// handlerFact returns the HandlerFact of a function of this package or of a dependency.
func (st *state) handlerFact(fn *types.Func) *HandlerFact {
	fn = fn.Origin()
	if fn.Pkg() == st.pass.Pkg {
		return st.handlers[fn]
	}
	var hf HandlerFact
	if st.pass.ImportObjectFact(fn, &hf) {
		return &hf
	}
	return nil
}

// route builds the Route of one call to a route function.
func (st *state) route(call *ast.CallExpr, rule routeRule) Route {
	r := Route{Method: rule.method, Pos: st.pass.Fset.Position(call.Pos()).String()}
	if rule.pathArg >= len(call.Args) || rule.handlerArg >= len(call.Args) {
		r.Undocumented = "malformed call: fewer arguments than the route rule names"
		return r
	}
	tv, ok := st.pass.TypesInfo.Types[call.Args[rule.pathArg]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		r.Undocumented = "the path is not a constant string"
		return r
	}
	r.Path = constant.StringVal(tv.Value)

	h := st.handler(call.Args[rule.handlerArg])
	r.Handler = h.name
	r.Errmap = h.errmapName
	switch {
	case h.reason != "":
		r.Undocumented = h.reason
	case h.errmap == nil && h.errmapName != "":
		r.Undocumented = "errmap " + h.errmapName + " does not resolve to a function with a valid //effectgo:errmap"
	case h.errmap == nil:
		r.Undocumented = "no errmap: the handler has no //effectgo:handler directive"
	}
	if r.Undocumented != "" {
		return r
	}

	r.Complete = true
	for _, v := range h.set {
		code, ok := h.errmap.StatusOf(v)
		if !ok {
			r.Complete = false
		}
		r.Errors = append(r.Errors, RouteError{Status: code, Variant: v.Name, Via: v.Chain, Opaque: v.Kind == errset.KindOpaque || v.Kind == errset.KindFresh})
	}
	return r
}

// handlerInfo is the resolved handler argument of a route.
type handlerInfo struct {
	name       string
	set        []errset.Variant
	errmap     *errset.ErrmapFact
	errmapName string
	reason     string // why the handler cannot be resolved; "" when it can
}

// handler resolves the handler argument of a route. Three forms are followed:
// a call to a handler constructor (a function that returns func literals), a
// function (or a qualified function), and a func literal. Anything else is
// reported with a reason.
func (st *state) handler(e ast.Expr) handlerInfo {
	info := st.pass.TypesInfo
	e = unparen(e)
	switch x := e.(type) {
	case *ast.FuncLit:
		fe, ok := st.res.Literals[x]
		if !ok {
			return handlerInfo{name: "func literal", reason: "the func literal does not return an error"}
		}
		return handlerInfo{name: "func literal", set: fe.Variants,
			reason: "no errmap: a func literal cannot carry //effectgo:handler (name the handler function instead)"}

	case *ast.CallExpr:
		fn := typeutil.StaticCallee(info, x)
		if fn == nil {
			return handlerInfo{name: "call", reason: "the handler is a call that is not to a static function"}
		}
		hf := st.handlerFact(fn)
		name := fn.Name() + "(...)"
		switch {
		case hf == nil:
			return handlerInfo{name: name, reason: name + " returns no error set (not a handler constructor)"}
		case !hf.Handler:
			return handlerInfo{name: name, reason: name + " does not return a func literal, so its handler is not visible (it may wrap another handler)"}
		}
		return handlerInfo{name: name, set: hf.Set, errmap: hf.Errmap, errmapName: hf.ErrmapName}

	case *ast.Ident, *ast.SelectorExpr:
		var obj types.Object
		if id, ok := x.(*ast.Ident); ok {
			obj = info.Uses[id]
		} else {
			obj = info.Uses[x.(*ast.SelectorExpr).Sel]
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			return handlerInfo{name: exprText(x), reason: exprText(x) + " is not a declared function (a value)"}
		}
		hf := st.handlerFact(fn)
		if hf == nil {
			return handlerInfo{name: fn.Name(), reason: fn.Name() + " has no error set (no error result)"}
		}
		if hf.Handler {
			return handlerInfo{name: fn.Name(), reason: fn.Name() + " is a handler constructor: pass the result of calling it"}
		}
		return handlerInfo{name: fn.Name(), set: hf.Set, errmap: hf.Errmap, errmapName: hf.ErrmapName}
	}
	return handlerInfo{name: "expression", reason: "the handler is not a function, a call or a func literal"}
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

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}
