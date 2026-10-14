package errset

// HTTP status inference. An errmap (//effectgo:errmap) maps domain errors to
// HTTP errors. For every variant the errset analyzer knows, this file answers:
// which status does the variant produce once it reaches Fuego's error handler?
//
// The answer comes from three sources, in this order:
//
//  1. The StatusCode() int method of the variant's type, when every return
//     statement of its body yields the same constant (StatusFact, exported on
//     the method, so facts from dependencies such as fuego work).
//  2. A "status <type> <code>" stub line (internal/stubs), for a type whose
//     status cannot be inferred from its body.
//  3. Otherwise the status is dynamic and unknown. Callers must not guess a
//     code for it; the OpenAPI output uses the "default" response.
//
// A variant with no StatusCode method and no stub is a plain error: Fuego sends
// it as 500 (serialization.go, SendJSONError). An opaque variant (?) is treated
// the same way, see ownStatus.

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/types/typeutil"
)

// StatusFact is exported on a StatusCode() int method whose body returns the
// same constant on every path. Code is that constant.
type StatusFact struct {
	Code int
}

// AFact marks StatusFact as an analysis fact.
func (*StatusFact) AFact() {}

func (f *StatusFact) String() string { return fmt.Sprintf("errset-status: %d", f.Code) }

// Kinds of errmap branch: how the branch selects variants.
const (
	matchSentinel = "sentinel" // errors.Is(err, S) or err == S
	matchType     = "type"     // errors.As(err, &T) with a concrete T, or a type-switch case T
	matchIface    = "iface"    // errors.As(err, &I) with an interface I, or a type-switch case I
	matchAny      = "any"      // default, or a condition the analyzer cannot read
)

// Branch is one rule of an errmap, in the order in which the rules are tried.
// The first branch that matches a variant decides its status.
type Branch struct {
	Match       string   // one of the match constants
	Key         string   // sentinel or type key, or interface key ("" for an unnamed interface)
	Methods     []string // method set of an interface target
	Status      int      // the constant status the rule returns; 0 when unknown
	Passthrough bool     // the rule returns the error itself: each variant keeps its own status
	// Carrier narrows a passthrough: only a carried variant (Variant.Carried)
	// keeps its status; any other matching variant has an unknown status. Used by
	// DefaultErrmap, where the framework reads the status from a method.
	Carrier bool
}

// matches reports whether the branch selects variant v.
func (b Branch) matches(v Variant) bool {
	switch b.Match {
	case matchAny:
		return true
	case matchSentinel:
		return v.Kind == KindSentinel && v.Key == b.Key
	case matchType:
		return v.Kind == KindType && v.Key == b.Key
	case matchIface:
		if v.Kind == KindOpaque || v.Kind == KindParam || v.Kind == KindFresh {
			return false
		}
		if b.Key != "" && v.Key == ifacePrefix+b.Key {
			return true
		}
		return implementsAll(v.Methods, b.Methods)
	}
	return false
}

// ownStatus is the status of variant v when no errmap rule maps it. Fuego's
// error handler returns an error that implements ErrorWithStatus with that
// status (errors.go:198-209 and 237-239, serialization.go SendJSONError), and
// sends any other error as 500. An unknown status is reported as ok=false.
//
// An opaque variant (?) is 500: its type is not visible, so it cannot be shown
// to carry a status of its own, and an error without one is sent as 500. The
// status is fixed here rather than read from a stub, because Fuego's rule is the
// server's error handler (serialization.go:210-216), not a property of a type,
// and the errset facts do not say which server handles a handler. The unknown
// cases are the param and iface variants, and a dynamic status.
func ownStatus(v Variant) (int, bool) {
	switch {
	case v.Kind == KindFresh:
		return 500, true
	case v.Kind == KindOpaque && v.Status > 0:
		// a collapsed foreign variant keeps the status of its type
		return v.Status, true
	case v.Kind == KindOpaque && v.DynamicStatus:
		return 0, false
	case v.Kind == KindOpaque:
		return 500, true
	case v.Kind == KindParam, v.Kind == KindIface:
		return 0, false
	case v.Status > 0:
		return v.Status, true
	case v.DynamicStatus:
		return 0, false
	}
	return 500, true
}

// StatusOf returns the HTTP status that the errmap produces for variant v. ok
// is false when the status is unknown; the caller then uses the "default"
// response.
func (m *ErrmapFact) StatusOf(v Variant) (int, bool) {
	for _, b := range m.Branches {
		if !b.matches(v) {
			continue
		}
		if b.Passthrough {
			if b.Carrier {
				if v.Carried && v.Status > 0 {
					return v.Status, true
				}
				return 0, false
			}
			return ownStatus(v)
		}
		return b.Status, b.Status > 0
	}
	return ownStatus(v)
}

// isStatusSig reports whether t is func() int.
func isStatusSig(t types.Type) bool {
	sig, ok := t.(*types.Signature)
	return ok && sig.Params().Len() == 0 && sig.Results().Len() == 1 &&
		types.Identical(sig.Results().At(0).Type(), types.Typ[types.Int])
}

// constStatus returns the constant that every return statement of a
// StatusCode body yields. It fails when a return is not a constant, or when
// the constants differ (a dynamic status, as fuego.HTTPError has).
func constStatus(info *types.Info, body *ast.BlockStmt) (int, bool) {
	var rets []*ast.ReturnStmt
	walkOwn(body, func(n ast.Node) {
		if r, ok := n.(*ast.ReturnStmt); ok {
			rets = append(rets, r)
		}
	})
	code := 0
	for _, r := range rets {
		if len(r.Results) != 1 {
			return 0, false
		}
		tv, ok := info.Types[r.Results[0]]
		if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
			return 0, false
		}
		v, exact := constant.Int64Val(tv.Value)
		if !exact || v < 100 || v > 599 {
			return 0, false
		}
		if code != 0 && code != int(v) {
			return 0, false
		}
		code = int(v)
	}
	return code, code != 0
}

// collectStatus finds the constant StatusCode methods of this package and
// exports them as facts. It runs before the stub files are loaded.
func (p *pkgState) collectStatus() {
	for _, u := range p.units {
		if u.decl.Recv == nil || u.decl.Name.Name != "StatusCode" || !isStatusSig(u.sig) {
			continue
		}
		if code, ok := constStatus(p.pass.TypesInfo, u.decl.Body); ok {
			p.statusDecls[u.obj] = code
			p.pass.ExportObjectFact(u.obj, &StatusFact{Code: code})
		}
	}
}

// statusOfMethod returns the constant status of a StatusCode method, from this
// package or from the fact of a dependency.
func (p *pkgState) statusOfMethod(fn *types.Func) (int, bool) {
	fn = fn.Origin()
	if fn.Pkg() == p.pass.Pkg {
		code, ok := p.statusDecls[fn]
		return code, ok
	}
	var f StatusFact
	if p.pass.ImportObjectFact(fn, &f) {
		return f.Code, true
	}
	return 0, false
}

// statusMethod returns the StatusCode method of n, if it has one with the
// right signature.
func statusMethod(n *types.Named) (*types.Func, bool) {
	obj, _, _ := types.LookupFieldOrMethod(n, true, nil, "StatusCode")
	fn, ok := obj.(*types.Func)
	if !ok || !isStatusSig(fn.Type()) {
		return nil, false
	}
	return fn, true
}

// typeStatus returns the status that Fuego gives a value of named type n when
// no errmap rule maps it. code is 0 when the status is not a known constant;
// dynamic is then true when n has a StatusCode method (its status depends on
// the value, as fuego.HTTPError's does), and false for a plain error type.
func (p *pkgState) typeStatus(n *types.Named) (code int, dynamic bool) {
	if fn, ok := statusMethod(n); ok {
		if c, ok := p.statusOfMethod(fn); ok {
			return c, false
		}
	}
	if c, ok := p.statusStubs[objKey(n.Obj())]; ok {
		return c, false
	}
	_, hasMethod := statusMethod(n)
	return 0, hasMethod
}

// loadStatusStubs reads the status rules of the stub files into p.statusStubs.
// The key is the canonical type, so an alias resolves to its origin.
func (p *pkgState) loadStatusStubs(rules []statusRule) {
	p.statusStubs = map[string]int{}
	for _, r := range rules {
		pkg, typ := p.canonicalType(r.pkg, r.typ)
		p.statusStubs[pkg+"."+typ] = r.code
	}
}

// statusRule is a parsed status line.
type statusRule struct {
	pkg, typ string
	code     int
}

// fieldStatus reads a literal field named Status. present is true when the
// literal sets the field; code is 0 when the value is not a constant.
func fieldStatus(info *types.Info, lit *ast.CompositeLit) (code int, present bool) {
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Status" {
			continue
		}
		present = true
		tv, ok := info.Types[kv.Value]
		if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
			return 0, true
		}
		if v, exact := constant.Int64Val(tv.Value); exact && v >= 100 && v <= 599 {
			return int(v), true
		}
	}
	return 0, present
}

// literalStatus returns the status of a composite literal that is returned as
// an error. The order follows Fuego: a constant StatusCode method wins (the
// handler sets the status from it), then a Status field, then a plain type.
func (p *pkgState) literalStatus(lit *ast.CompositeLit) int {
	t := types.Unalias(p.pass.TypesInfo.TypeOf(lit))
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	n, named := t.(*types.Named)
	if !named {
		code, _ := fieldStatus(p.pass.TypesInfo, lit)
		return code
	}
	code, dynamic := p.typeStatus(n)
	if code > 0 {
		return code
	}
	if fc, present := fieldStatus(p.pass.TypesInfo, lit); present {
		return fc
	}
	if dynamic {
		return 0
	}
	return 500
}

// exprStatus returns the status of one returned expression of an errmap. It
// reports passthrough when the expression is the error itself (the parameter,
// or the case variable of a type switch).
func (p *pkgState) exprStatus(e ast.Expr, param, bound types.Object) (code int, passthrough bool) {
	e = unparen(e)
	info := p.pass.TypesInfo
	var obj types.Object
	switch x := e.(type) {
	case *ast.Ident:
		obj = info.Uses[x]
	case *ast.SelectorExpr:
		obj = info.Uses[x.Sel]
	case *ast.CompositeLit:
		return p.literalStatus(x), false
	case *ast.UnaryExpr:
		if lit, ok := unparen(x.X).(*ast.CompositeLit); ok && x.Op == token.AND {
			return p.literalStatus(lit), false
		}
	case *ast.CallExpr:
		// return huma.Error404NotFound(…): the status the call carries (carried.go)
		code, _ := p.callCarriedCode(x)
		return code, false
	}
	if obj != nil && (obj == param || obj == bound) {
		return 0, true
	}
	if _, ok := sentinelKey(obj); ok {
		return 500, false // a sentinel is a plain error (its value is not inspected)
	}
	return 0, false
}

// returnsStatus combines the statuses of the return statements of one branch.
// All passthrough gives a passthrough branch; a mix or an unknown gives 0.
func (p *pkgState) returnsStatus(rets []*ast.ReturnStmt, param, bound types.Object) (int, bool) {
	if len(rets) == 0 {
		return 0, false
	}
	code, pass := 0, 0
	for _, r := range rets {
		if len(r.Results) != 1 {
			return 0, false
		}
		c, passthrough := p.exprStatus(r.Results[0], param, bound)
		if passthrough {
			pass++
			continue
		}
		if c == 0 || (code != 0 && code != c) {
			return 0, false
		}
		code = c
	}
	switch {
	case pass == len(rets):
		return 0, true
	case pass == 0:
		return code, false
	}
	return 0, false
}

// branchBuilder reads the top-level rules of an errmap body in order.
type branchBuilder struct {
	p     *pkgState
	param types.Object
	out   []Branch
	def   *Branch // the else or default rule, which is tried last
	done  bool    // an unconditional return ends the rules
}

// emit records a rule. Its status comes from the returns in body.
func (b *branchBuilder) emit(m Branch, body []ast.Stmt, bound types.Object) {
	var rets []*ast.ReturnStmt
	for _, s := range body {
		walkOwn(s, func(n ast.Node) {
			if r, ok := n.(*ast.ReturnStmt); ok {
				rets = append(rets, r)
			}
		})
	}
	m.Status, m.Passthrough = b.p.returnsStatus(rets, b.param, bound)
	if m.Match == matchAny {
		b.def = &m
		return
	}
	b.out = append(b.out, m)
}

// condBranch reads one condition. ok is false when the condition is not a
// recognised error test; the caller then treats the rule as matchAny.
func (b *branchBuilder) condBranch(cond ast.Expr) (Branch, bool) {
	info := b.p.pass.TypesInfo
	switch x := unparen(cond).(type) {
	case *ast.CallExpr:
		f := typeutil.StaticCallee(info, x)
		if f == nil || f.Pkg() == nil || f.Pkg().Path() != "errors" || len(x.Args) != 2 || !b.isParam(x.Args[0]) {
			return Branch{}, false
		}
		switch f.Name() {
		case "Is":
			if k, ok := sentinelKeyOf(info, x.Args[1]); ok {
				return Branch{Match: matchSentinel, Key: k}, true
			}
		case "As":
			u, ok := unparen(x.Args[1]).(*ast.UnaryExpr)
			if !ok {
				return Branch{}, false
			}
			return b.targetBranch(info.TypeOf(u.X))
		}
	case *ast.BinaryExpr:
		if x.Op != token.EQL {
			return Branch{}, false
		}
		switch {
		case b.isParam(x.X):
			if k, ok := sentinelKeyOf(info, x.Y); ok {
				return Branch{Match: matchSentinel, Key: k}, true
			}
		case b.isParam(x.Y):
			if k, ok := sentinelKeyOf(info, x.X); ok {
				return Branch{Match: matchSentinel, Key: k}, true
			}
		}
	}
	return Branch{}, false
}

// targetBranch is the rule for an errors.As target of type t (a pointer to a
// concrete error type or to an interface).
func (b *branchBuilder) targetBranch(t types.Type) (Branch, bool) {
	if t == nil {
		return Branch{}, false
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	if types.IsInterface(t) {
		m := Branch{Match: matchIface, Methods: methodsOf(t)}
		if n, ok := t.(*types.Named); ok {
			m.Key = objKey(n.Obj())
		}
		return m, true
	}
	if n := namedErrorType(t); n != nil {
		return Branch{Match: matchType, Key: objKey(n.Obj())}, true
	}
	return Branch{}, false
}

func (b *branchBuilder) isParam(e ast.Expr) bool {
	id, ok := unparen(e).(*ast.Ident)
	if !ok || b.param == nil {
		return false
	}
	return b.p.pass.TypesInfo.Uses[id] == b.param
}

// initBranch reads the errors.AsType form of an if statement (see asTypeInit),
// on the error parameter, as condBranch reads errors.As.
func (b *branchBuilder) initBranch(s *ast.IfStmt) (Branch, bool) {
	subject, targ, ok := asTypeInit(b.p.pass.TypesInfo, s.Init, s.Cond)
	if !ok || !b.isParam(subject) {
		return Branch{}, false
	}
	return b.targetBranch(targ)
}

// ifChain reads an if / else-if / else chain. Every condition is a rule; the
// final else is the default.
func (b *branchBuilder) ifChain(s *ast.IfStmt) {
	for s != nil && !b.done {
		m, ok := b.condBranch(s.Cond)
		if !ok {
			m, ok = b.initBranch(s)
		}
		if !ok {
			m = Branch{Match: matchAny}
		}
		b.emit(m, s.Body.List, nil)
		switch e := s.Else.(type) {
		case *ast.IfStmt:
			s = e
		case *ast.BlockStmt:
			b.emit(Branch{Match: matchAny}, e.List, nil)
			return
		default:
			return
		}
	}
}

// switchStmt reads an expression switch. Without a tag, each case is a
// condition; with the error parameter as tag, each case is a sentinel.
func (b *branchBuilder) switchStmt(s *ast.SwitchStmt) {
	for _, st := range s.Body.List {
		cc := st.(*ast.CaseClause)
		if cc.List == nil {
			b.emit(Branch{Match: matchAny}, cc.Body, nil)
			continue
		}
		for _, e := range cc.List {
			var m Branch
			var ok bool
			switch {
			case s.Tag == nil:
				m, ok = b.condBranch(e)
			case b.isParam(s.Tag):
				var k string
				k, ok = sentinelKeyOf(b.p.pass.TypesInfo, e)
				m = Branch{Match: matchSentinel, Key: k}
			}
			if !ok {
				m = Branch{Match: matchAny}
			}
			b.emit(m, cc.Body, nil)
		}
	}
}

// typeSwitch reads switch e := err.(type). Each case type is a rule; the case
// variable is the passthrough.
func (b *branchBuilder) typeSwitch(s *ast.TypeSwitchStmt) {
	info := b.p.pass.TypesInfo
	x := typeSwitchSubject(s)
	for _, st := range s.Body.List {
		cc := st.(*ast.CaseClause)
		bound := info.Implicits[cc]
		if x == nil || !b.isParam(x) || cc.List == nil {
			b.emit(Branch{Match: matchAny}, cc.Body, bound)
			continue
		}
		for _, e := range cc.List {
			m, ok := b.targetBranch(info.TypeOf(e))
			if !ok {
				m = Branch{Match: matchAny}
			}
			b.emit(m, cc.Body, bound)
		}
	}
}

// typeSwitchSubject returns the expression x of switch v := x.(type), or nil.
func typeSwitchSubject(s *ast.TypeSwitchStmt) ast.Expr {
	var a ast.Expr
	switch st := s.Assign.(type) {
	case *ast.AssignStmt:
		if len(st.Rhs) == 1 {
			a = st.Rhs[0]
		}
	case *ast.ExprStmt:
		a = st.X
	}
	if ta, ok := unparen(a).(*ast.TypeAssertExpr); ok {
		return ta.X
	}
	return nil
}

// errmapBranches reads the rules of an errmap in order. The default rule, if
// any, is last, as in a Go switch.
func (p *pkgState) errmapBranches(u *fnUnit) []Branch {
	b := &branchBuilder{p: p}
	if fl := u.decl.Type.Params.List; len(fl) > 0 && len(fl[0].Names) > 0 {
		b.param = p.pass.TypesInfo.Defs[fl[0].Names[0]]
	}
	for _, s := range u.decl.Body.List {
		if b.done {
			break
		}
		switch x := s.(type) {
		case *ast.IfStmt:
			b.ifChain(x)
		case *ast.SwitchStmt:
			b.switchStmt(x)
		case *ast.TypeSwitchStmt:
			b.typeSwitch(x)
		case *ast.ReturnStmt:
			b.emit(Branch{Match: matchAny}, []ast.Stmt{x}, nil)
			b.done = true
		}
	}
	out := b.out
	if b.def != nil && !b.done {
		out = append(out, *b.def)
	}
	return out
}

// sentinelKeyOf returns the key of the sentinel named by e, if e names one.
func sentinelKeyOf(info *types.Info, e ast.Expr) (string, bool) {
	var obj types.Object
	switch x := unparen(e).(type) {
	case *ast.Ident:
		obj = info.Uses[x]
	case *ast.SelectorExpr:
		obj = info.Uses[x.Sel]
	}
	if v, ok := sentinelKey(obj); ok {
		return objKey(v), true
	}
	return "", false
}

// statusTrace renders "code name" pairs, sorted by code, for the -errset.statuses report.
func statusTrace(m *ErrmapFact, vs []Variant) string {
	type row struct {
		code int
		text string
	}
	var rows []row
	for _, v := range displayOrder(vs) {
		code, ok := m.StatusOf(v)
		text := v.Name
		if !ok {
			rows = append(rows, row{1000, "default " + text})
			continue
		}
		rows = append(rows, row{code, fmt.Sprintf("%d %s", code, text)})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].code < rows[j].code })
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, r.text)
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, " | ")
}

// handlerOf returns the //effectgo:handler directive of u, if it has one.
func handlerOf(u *fnUnit) (directive, bool) {
	for _, d := range directivesOf(u.decl.Doc) {
		if d.kind == "handler" {
			return d, true
		}
	}
	return directive{}, false
}

// reportStatuses prints the status of each variant of a handler, through its
// errmap, for the -errset.statuses flag. Resolution failures are reported by
// checkHandler, so they are silent here.
func (p *pkgState) reportStatuses(u *fnUnit, d directive) {
	obj, ok := p.resolve(d.arg, u.decl.Pos()).(*types.Func)
	if !ok {
		return
	}
	var m ErrmapFact
	if !p.pass.ImportObjectFact(obj, &m) {
		return
	}
	vs := p.work[u.obj].effective().list()
	p.pass.Reportf(u.decl.Name.Pos(), "statuses of %s: %s", funcName(u.obj), statusTrace(&m, vs))
}
