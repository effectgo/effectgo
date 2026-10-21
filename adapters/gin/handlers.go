package ginhttp

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/effectgo/effectgo/errset"
)

// handlersIn analyzes a declaration's own body when it is a handler, and every
// func literal with a *gin.Context parameter inside it. The literals share the
// declaration's flow, so their local error variables are the enclosing ones.
func (a *analyzer) handlersIn(fd *ast.FuncDecl) {
	flow := a.errRes.Flows[fd]
	if ginCtxIndex(a.info, fd.Type) >= 0 {
		a.analyzeBody(fd.Type, fd.Body, flow, funcLabel(fd), a.posStr(fd.Name.Pos()))
	}
	a.literalsIn(fd, flow, funcLabel(fd))
}

// literalsIn analyzes every handler literal under node. encl names the enclosing
// declaration, which is part of the label of each literal. A literal outside any
// declaration (a package-level variable) has no flow, so its variables are unknown.
func (a *analyzer) literalsIn(node ast.Node, flow errset.Flow, encl string) {
	ast.Inspect(node, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok && ginCtxIndex(a.info, lit.Type) >= 0 {
			a.analyzeBody(lit.Type, lit.Body, flow, literalLabel(encl), a.posStr(lit.Pos()))
		}
		return true
	})
}

// literalLabel is the display name of a func literal inside the named declaration.
func literalLabel(encl string) string {
	return "func literal in " + encl
}

// analyzeBody runs the rule engine on one handler body and records its findings.
// name and pos label the handler (pos is file:line).
func (a *analyzer) analyzeBody(ft *ast.FuncType, body *ast.BlockStmt, flow errset.Flow, name, pos string) {
	a.res.Handlers++
	params := map[types.Object]bool{}
	for _, o := range paramObjs(a.info, ft) {
		if o != nil {
			params[o] = true
		}
	}
	e := newEngine(a, flow, params)
	e.walk(body.List)
	a.findings = append(a.findings, e.finalize(name, pos)...)
	// Sink mapper calls are whole answers: each one decides the statuses of its
	// error argument's variants. Literals with their own *gin.Context are skipped:
	// they are analyzed on their own.
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok && ginCtxIndex(a.info, lit.Type) >= 0 {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			a.mapperCall(call, flow, name, pos)
		}
		return true
	})
}

// mapperCall reports the variants of a mapper call's error argument that reach a
// 5xx status through the mapper.
func (a *analyzer) mapperCall(call *ast.CallExpr, flow errset.Flow, name, pos string) {
	m := a.sinkMapperOfCall(call)
	if m == nil || m.Err >= len(call.Args) {
		return
	}
	mapper := a.calleeName(call)
	for _, v := range flow.Of(call.Args[m.Err]) {
		st, how, ok := m.statusOf(v)
		if !ok || st < 500 {
			continue
		}
		a.findings = append(a.findings, a.reachFinding(name, pos, call.Pos(), v, st, "sink mapper "+mapper+", "+how))
	}
}

// statusOf returns the status a mapper gives variant v: its first matching
// branch, else its default. ok is false when no status is known.
func (m *sinkMapperFact) statusOf(v errset.Variant) (int, string, bool) {
	for _, b := range m.Branches {
		if b.Br.Matches(v) {
			return b.Status, "branch at " + b.Pos, b.Status != 0
		}
	}
	if m.HasDefault {
		return m.Default, "the mapper's default", m.Default != 0
	}
	return 0, "", false
}

// reachFinding builds a reach finding: variant v reaches status st at the sink
// (or mapper call) at pos, inside the handler named name at handlerPos.
func (a *analyzer) reachFinding(name, handlerPos string, pos token.Pos, v errset.Variant, st int, how string) Finding {
	f := Finding{
		Kind:       "reach",
		Handler:    name + " (" + handlerPos + ")",
		HandlerPos: handlerPos,
		Pos:        a.posStr(pos),
		Variant:    v.Name,
		ErrKind:    v.Kind,
		Status:     st,
		Via:        viaText(v.Chain) + "; " + how,
	}
	if v.Kind == errset.KindSentinel || v.Kind == errset.KindType {
		f.Key = v.Key
		f.Project = inModule(keyPkgOf(v.Key), a.module)
	}
	return f
}

// viaText renders a witness chain.
func viaText(chain []string) string {
	if len(chain) == 0 {
		return "(direct)"
	}
	return strings.Join(chain, " -> ")
}

// objOf returns the object of an identifier.
func (a *analyzer) objOf(id *ast.Ident) types.Object {
	if id == nil {
		return nil
	}
	if o := a.info.Uses[id]; o != nil {
		return o
	}
	return a.info.Defs[id]
}

// calleeName returns the name of a call's callee, for display.
func (a *analyzer) calleeName(call *ast.CallExpr) string {
	if f := calleeOf(a.info, call); f != nil {
		return f.Name()
	}
	return types.ExprString(call.Fun)
}

// sinkMapperOfCall returns the sink mapper that a call invokes: a local one, or
// one exported by the analysis of its package.
func (a *analyzer) sinkMapperOfCall(call *ast.CallExpr) *sinkMapperFact {
	f := calleeOf(a.info, call)
	if f == nil {
		return nil
	}
	if m := a.local[f]; m != nil {
		return m
	}
	var m sinkMapperFact
	if a.pass.ImportObjectFact(f, &m) {
		return &m
	}
	return nil
}

// statusMapperOfCall returns the status mapper that a call invokes.
func (a *analyzer) statusMapperOfCall(call *ast.CallExpr) *statusMapperFact {
	f := calleeOf(a.info, call)
	if f == nil {
		return nil
	}
	if s := a.localS[f]; s != nil {
		return s
	}
	var s statusMapperFact
	if a.pass.ImportObjectFact(f, &s) {
		return &s
	}
	return nil
}

// factoryOf returns the handler literal of a factory function, if any.
func (a *analyzer) factoryOf(f *types.Func) string {
	if f == nil {
		return ""
	}
	if x := a.localF[f]; x != nil {
		return x.LitPos
	}
	var x factoryFact
	if a.pass.ImportObjectFact(f, &x) {
		return x.LitPos
	}
	return ""
}

// constString returns the value of a constant string expression.
func constString(info *types.Info, e ast.Expr) (string, bool) {
	tv, ok := info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// constBool reports whether e is the constant true.
func constBool(info *types.Info, e ast.Expr) (bool, bool) {
	tv, ok := info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(tv.Value), true
}

// sameObj reports whether expression e is the identifier of object obj.
func sameObj(info *types.Info, e ast.Expr, obj types.Object) bool {
	_, o := ident(info, e)
	return o != nil && o == obj
}
