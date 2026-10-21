package errset

// Route checks. A function registered through a route rule of the stub files
// (`route <target> <METHOD> <path> <handler>`, stubs/rt.stubs for rt/web) is an
// HTTP handler, with no //effectgo:handler directive needed. The framework
// encodes an error by its own StatusCode and sends any other error as 500 (see
// rt/web ErrorBody). So every variant of the handler's error set must have a
// constant status of its own, and this check reports the variants that do not.
//
// The status rule is the framework's, not an errmap's: a variant keeps the
// StatusCode of its own type (StatusFact), as a passthrough errmap does, and
// there is no fallback that could give an unhandled variant a status.
//
// The checks run in every run of the analyzer (default, go vet and the editor
// plugin). A variant with an unknown origin (`?`, an unresolved parameter, an
// interface) is reported only in contract mode: the handler carries an
// //effectgo:errors directive. A closure is reported, because its set is not
// inferred (see docs/design/framework.md).

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/effectgo/effectgo/internal/stubs"
)

// RouteRule is one route rule: the method, the index of the path argument and
// the index of the handler argument.
type RouteRule struct {
	Method     string
	PathArg    int
	HandlerArg int
}

// RouteRules reads the route rules of the stub files, keyed by the registering
// function (pkgpath.Name). Shared by errset and tsgen.
func RouteRules(rules []stubs.Rule) map[string]RouteRule {
	out := map[string]RouteRule{}
	for _, r := range rules {
		if r.Verb != stubs.VerbRoute || len(r.Args) < 2 {
			continue
		}
		if _, isField, _ := stubs.ParseFieldRef(r.Value); isField {
			continue // the method is a field of an argument: not a constant (huma)
		}
		path, err := stubs.ParseRouteArg(r.Args[0])
		if err != nil || path.Field != nil {
			continue // checked by internal/stubs; a path field is not read here
		}
		h, err := strconv.Atoi(r.Args[1])
		if err != nil {
			continue
		}
		out[r.Target.Pkg+"."+r.Target.Name] = RouteRule{Method: r.Value, PathArg: path.Index, HandlerArg: h}
	}
	return out
}

// RouteCallRule returns the route rule of a call to a registering function, when
// the call has the handler argument the rule names.
func RouteCallRule(info *types.Info, call *ast.CallExpr, table map[string]RouteRule) (RouteRule, bool) {
	fn := typeutil.StaticCallee(info, call)
	if fn == nil || fn.Pkg() == nil {
		return RouteRule{}, false
	}
	fn = fn.Origin()
	rule, ok := table[fn.Pkg().Path()+"."+fn.Name()]
	if !ok || rule.HandlerArg < 0 || rule.HandlerArg >= len(call.Args) {
		return RouteRule{}, false
	}
	return rule, true
}

// HandledStatus is the constant error status that a route check gives variant v:
// ok is true when routeVerdict classes the variant as handled.
func HandledStatus(v Variant) (int, bool) {
	if routeVerdict(v) != routeHandled {
		return 0, false
	}
	return v.Status, true
}

// loadRoutes reads the route rules of the stub files of this package.
func (p *pkgState) loadRoutes(rules []stubs.Rule) {
	p.routeRules = RouteRules(rules)
}

// routeKind is how a variant of a route's error set relates to its status.
type routeKind int

const (
	// routeHandled: the variant has a constant error status of its own.
	routeHandled routeKind = iota
	// routeUnknown: the variant's origin is not known (see routeVerdict); it is
	// reported only in contract mode.
	routeUnknown
	// routeNoStatus: the variant provably has no constant error status.
	routeNoStatus
)

// routeVerdict classifies variant v for a route. It follows the status facts of
// errset: a constant StatusCode in 400 to 599 is kept, a dynamic one or none is
// not. An opaque, unresolved or interface variant has no known origin, so its
// status is not known either.
func routeVerdict(v Variant) routeKind {
	switch v.Kind {
	case KindOpaque, KindParam, KindIface:
		return routeUnknown
	case KindFresh:
		return routeNoStatus
	case KindSentinel:
		// A sentinel's status is that of its initializer, which errset does not
		// evaluate. A sentinel whose type has a StatusCode method is not known.
		if hasStatusMethod(v.Methods) {
			return routeUnknown
		}
		return routeNoStatus
	}
	if v.DynamicStatus {
		return routeNoStatus
	}
	if v.Status >= 400 && v.Status <= 599 {
		return routeHandled
	}
	return routeNoStatus
}

// hasStatusMethod reports whether a method set has StatusCode() int.
func hasStatusMethod(methods []string) bool {
	for _, m := range methods {
		if m == "StatusCode()(int)" {
			return true
		}
	}
	return false
}

// routeName is the name of variant v in a route message.
func routeName(v Variant) string {
	switch v.Kind {
	case KindOpaque:
		return "an unclassified error"
	case KindParam:
		return "an unresolved " + v.Name + " error"
	case KindFresh:
		return "fresh error " + v.Name
	}
	return v.Name
}

// checkRoutes reports the route registrations of this package whose handler
// cannot be given a status for each variant of its error set.
func (p *pkgState) checkRoutes() {
	if len(p.routeRules) == 0 {
		return
	}
	for _, f := range p.pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			rule, ok := RouteCallRule(p.pass.TypesInfo, call, p.routeRules)
			if !ok {
				return true
			}
			p.checkRoute(call, rule)
			return true
		})
	}
}

// checkRoute checks one route call.
func (p *pkgState) checkRoute(call *ast.CallExpr, rule RouteRule) {
	arg := call.Args[rule.HandlerArg]
	if _, ok := unparen(arg).(*ast.FuncLit); ok {
		if isTestFile(p.pass, call.Pos()) {
			return // a test registers closures to test the framework (see the doc comment)
		}
		p.pass.Reportf(call.Pos(), "route %s: handler is a function literal; register a named function or method value so its error set can be checked", p.routeText(call, rule))
		return
	}
	fn, ok := HandlerFunc(p.pass.TypesInfo, arg)
	if !ok {
		return // a variable, a field or a call: the handler is not known
	}
	var fact ErrFact
	if !p.pass.ImportObjectFact(fn, &fact) {
		return // no body in the analyzed graph (an interface method, a stub only)
	}
	set := fact.Errors
	if fact.HasHandler {
		set = fact.Handler
	}
	for _, v := range displayOrder(p.visible(set)) {
		switch routeVerdict(v) {
		case routeHandled:
		case routeUnknown:
			if fact.Contract && !ignoreOpaqueFlag {
				p.reportRoute(call, rule, fn, v)
			}
		case routeNoStatus:
			if v.Kind == KindFresh && ignoreOpaqueFlag {
				continue
			}
			p.reportRoute(call, rule, fn, v)
		}
	}
}

// reportRoute reports variant v of the handler fn of a route call.
func (p *pkgState) reportRoute(call *ast.CallExpr, rule RouteRule, fn *types.Func, v Variant) {
	p.pass.Reportf(call.Pos(), "route %s: handler %s may return %s%s, which has no constant StatusCode → HTTP 500",
		p.routeText(call, rule), funcName(fn), routeName(v), via(v.Chain))
}

// routeText is the method and path of a route call: "GET /api/contact". A path
// that is not a constant is written as its source text.
func (p *pkgState) routeText(call *ast.CallExpr, rule RouteRule) string {
	if rule.PathArg >= len(call.Args) {
		return rule.Method + " ?"
	}
	e := call.Args[rule.PathArg]
	if tv, ok := p.pass.TypesInfo.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return rule.Method + " " + constant.StringVal(tv.Value)
	}
	return rule.Method + " " + types.ExprString(e)
}

// HandlerFunc returns the declared function that a handler argument names: a
// named function, a method value, or a package-qualified function, with explicit
// type arguments removed. It is the origin of a generic function.
func HandlerFunc(info *types.Info, e ast.Expr) (*types.Func, bool) {
	var f *types.Func
	switch x := unparen(e).(type) {
	case *ast.IndexExpr:
		return HandlerFunc(info, x.X)
	case *ast.IndexListExpr:
		return HandlerFunc(info, x.X)
	case *ast.Ident:
		f, _ = info.Uses[x].(*types.Func)
	case *ast.SelectorExpr:
		if sel := info.Selections[x]; sel != nil {
			if sel.Kind() != types.MethodVal {
				return nil, false
			}
			f, _ = sel.Obj().(*types.Func)
		} else {
			f, _ = info.Uses[x.Sel].(*types.Func)
		}
	default: // any other expression is not a function value
	}
	if f == nil {
		return nil, false
	}
	return f.Origin(), true
}

// isTestFile reports whether pos is in a _test.go file. A closure in a test is a
// fixture of the test, not a handler the program serves, so it is not reported.
func isTestFile(pass *analysis.Pass, pos token.Pos) bool {
	return strings.HasSuffix(pass.Fset.Position(pos).Filename, "_test.go")
}

// hasDirective reports whether u's doc comment has the directive kind.
func hasDirective(u *fnUnit, kind string) bool {
	for _, d := range directivesOf(u.decl.Doc) {
		if d.kind == kind {
			return true
		}
	}
	return false
}
