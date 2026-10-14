package ginhttp

// The rule engine. It reads one function body (a handler, a func literal, or a
// sink mapper) and collects two kinds of rule per error variable:
//
//   - variant rules: "errors.Is(e, X) / errors.As / e == X" selects a branch
//     whose body answers with a constant status (or a status mapper's status);
//   - default rules: the status of a catch-all answer: "if e != nil { sink }",
//     the else of a test, a switch's default, or the sink that follows an
//     if-return on the same block ("fallthrough").
//
// The rules are gathered in source order. A variant takes the first matching
// variant rule, else the default of its variable, else no status (the variant
// is not reported). A rule or default is read at its own use: the variants of
// the variable at that identifier (errset.Flow.Of), so a guard above a sink
// removes the variants it tests before the sink answers them.

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/faintghost/effectgo/errset"
)

// varRule gives a status to the variants of one variable that match a branch.
type varRule struct {
	id         *ast.Ident // the use where the rule is read
	subj       types.Object
	br         errset.Branch
	status     int
	pos        token.Pos
	how        string
	fromMapper bool
}

// defRule gives the status of every variant of a variable without a variant rule.
type defRule struct {
	id     *ast.Ident // the use where the default is read
	status int
	pos    token.Pos
	how    string
}

type engine struct {
	a       *analyzer
	flow    errset.Flow
	params  map[types.Object]bool
	idents  map[types.Object]*ast.Ident
	vrs     []varRule
	defs    map[types.Object]defRule
	order   []types.Object
	pending []types.Object
	// pendID is the use of each pending variable at its guard (the fallback
	// use of a sink that does not name the variable).
	pendID map[types.Object]*ast.Ident
}

func newEngine(a *analyzer, flow errset.Flow, params map[types.Object]bool) *engine {
	return &engine{a: a, flow: flow, params: params, idents: map[types.Object]*ast.Ident{}, defs: map[types.Object]defRule{}, pendID: map[types.Object]*ast.Ident{}}
}

// note records a subject in order of first appearance.
func (e *engine) note(id *ast.Ident, obj types.Object) {
	if _, ok := e.idents[obj]; !ok {
		e.idents[obj] = id
		e.order = append(e.order, obj)
	}
}

// subjectOf returns the identifier and object of a subject expression.
func (e *engine) subjectOf(x ast.Expr) (*ast.Ident, types.Object) {
	id, obj := ident(e.a.info, x)
	if id == nil || obj == nil {
		return nil, nil
	}
	if _, ok := obj.(*types.Var); !ok {
		return nil, nil
	}
	return id, obj
}

// addDef sets the default of a variable, once: the first catch-all in source
// order decides. A status of 0 (unknown) also counts, so it blocks later defaults.
func (e *engine) addDef(id *ast.Ident, obj types.Object, st int, pos token.Pos, how string) {
	e.note(id, obj)
	if _, ok := e.defs[obj]; ok {
		return
	}
	e.defs[obj] = defRule{id: id, status: st, pos: pos, how: how}
}

// addPending marks a variable whose variant rule returns: a sink that follows
// on the same block answers the variants this rule did not select.
func (e *engine) addPending(obj types.Object, id *ast.Ident) {
	for _, p := range e.pending {
		if p == obj {
			return
		}
	}
	e.pending = append(e.pending, obj)
	e.pendID[obj] = id
}

// walk reads a statement list. Pending fallthroughs are scoped to the list: a
// sink in a nested block does not answer the variants of an outer if-return.
func (e *engine) walk(list []ast.Stmt) {
	saved := e.pending
	e.pending = nil
	defer func() { e.pending = saved }()
	for _, s := range list {
		switch x := s.(type) {
		case *ast.IfStmt:
			e.ifChain(x, nil)
		case *ast.SwitchStmt:
			e.switchStmt(x)
			for _, cc := range x.Body.List {
				e.walk(cc.(*ast.CaseClause).Body)
			}
		case *ast.TypeSwitchStmt:
			for _, cc := range x.Body.List {
				e.walk(cc.(*ast.CaseClause).Body)
			}
		case *ast.ExprStmt:
			if call, ok := ast.Unparen(x.X).(*ast.CallExpr); ok {
				if r, ok := e.a.sinkOf(call); ok {
					st := e.a.statusOfCall(call, r)
					for _, obj := range e.pending {
						// The sink answers the variable at its own use when it names it.
						id := identOfObj(e.a.info, call, obj)
						if id == nil {
							id = e.pendID[obj]
						}
						e.addDef(id, obj, st, call.Pos(), "the sink after the if-return")
					}
					e.pending = nil
				}
			}
		case *ast.BlockStmt:
			e.walk(x.List)
		case *ast.ForStmt:
			e.walk(x.Body.List)
		case *ast.RangeStmt:
			e.walk(x.Body.List)
		case *ast.LabeledStmt:
			e.walk([]ast.Stmt{x.Stmt})
		}
	}
}

// blockStatus reads the sink calls that are direct statements of a block. ok is
// false when there is none; a non-constant status is ok with status 0.
func (e *engine) blockStatus(list []ast.Stmt) (int, token.Pos, bool) {
	for _, s := range list {
		es, ok := s.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := ast.Unparen(es.X).(*ast.CallExpr)
		if !ok {
			continue
		}
		if r, ok := e.a.sinkOf(call); ok {
			return e.a.statusOfCall(call, r), call.Pos(), true
		}
	}
	return 0, token.NoPos, false
}

// terminates reports whether a block ends with a return.
func terminates(b *ast.BlockStmt) bool {
	if b == nil || len(b.List) == 0 {
		return false
	}
	_, ok := b.List[len(b.List)-1].(*ast.ReturnStmt)
	return ok
}

// addRule adds a variant rule. A direct rule (a sink in the function) is also
// an explicit mapping, so it is exported as an observation when it is a named
// variant with a known status.
func (e *engine) addRule(id *ast.Ident, obj types.Object, br errset.Branch, st int, pos token.Pos, how string, fromMapper bool) {
	e.note(id, obj)
	e.vrs = append(e.vrs, varRule{id: id, subj: obj, br: br, status: st, pos: pos, how: how, fromMapper: fromMapper})
	if !fromMapper && st > 0 && (br.Match == "sentinel" || br.Match == "type") && br.Key != "" {
		e.a.res.Obs = append(e.a.res.Obs, Obs{Key: br.Key, Status: st, Pos: e.a.posStr(pos)})
	}
}

// ifChain reads an if / else-if / else chain. chain is the subject of the
// whole chain (set by its first condition), or nil.
func (e *engine) ifChain(s *ast.IfStmt, chain *ast.Ident) {
	info := e.a.info
	// pendObj is the variable whose unselected variants the sink after this
	// if-return answers; it is added once the body is read, so that the sinks
	// of the body do not consume it.
	var pendObj types.Object
	var pendID *ast.Ident
	if s.Init != nil {
		if as, ok := s.Init.(*ast.AssignStmt); ok && len(as.Lhs) == 2 && len(as.Rhs) == 1 {
			if call, ok := as.Rhs[0].(*ast.CallExpr); ok && len(call.Args) > 0 {
				if sm := e.a.statusMapperOfCall(call); sm != nil {
					if id, obj := e.subjectOf(call.Args[0]); obj != nil {
						for _, b := range sm.Branches {
							e.addRule(id, obj, b.Br, b.Status, call.Pos(), "status mapper "+e.a.calleeName(call), true)
						}
						if terminates(s.Body) && s.Else == nil {
							pendObj, pendID = obj, id
						}
						if chain == nil {
							chain = id
						}
					}
				}
			}
		}
	}
	if sub, br, ok := errset.IfErrorTest(info, s); ok {
		if id, obj := e.subjectOf(sub); obj != nil {
			if st, pos, has := e.blockStatus(s.Body.List); has {
				e.addRule(id, obj, br, st, pos, "inline test (if-branch) at "+e.a.posStr(s.Pos()), false)
				if terminates(s.Body) && s.Else == nil {
					pendObj, pendID = obj, id
				}
			}
			if chain == nil {
				chain = id
			}
		}
	} else if bin, ok := ast.Unparen(s.Cond).(*ast.BinaryExpr); ok && (bin.Op == token.NEQ || bin.Op == token.EQL) {
		// err != nil: the body is the catch-all of the error. err == nil: the
		// else is the catch-all.
		var sub ast.Expr
		switch {
		case isNil(info, bin.Y):
			sub = bin.X
		case isNil(info, bin.X):
			sub = bin.Y
		}
		if sub != nil {
			if id, obj := e.subjectOf(sub); obj != nil && errset.IsError(info.TypeOf(sub)) {
				if bin.Op == token.NEQ {
					if st, pos, has := e.blockStatus(s.Body.List); has {
						e.addDef(id, obj, st, pos, "inline `err != nil` at "+e.a.posStr(s.Pos()))
					}
				} else if els, ok := s.Else.(*ast.BlockStmt); ok {
					if st, pos, has := e.blockStatus(els.List); has {
						e.addDef(id, obj, st, pos, "inline `err == nil` else at "+e.a.posStr(s.Pos()))
					}
				}
			}
		}
	}
	e.walk(s.Body.List)
	if pendObj != nil {
		e.addPending(pendObj, pendID)
	}
	switch el := s.Else.(type) {
	case *ast.IfStmt:
		e.ifChain(el, chain)
	case *ast.BlockStmt:
		if chain != nil {
			if obj := e.a.objOf(chain); obj != nil && !isNilTest(s.Cond) {
				if st, pos, has := e.blockStatus(el.List); has {
					e.addDef(chain, obj, st, pos, "else of the test at "+e.a.posStr(s.Pos()))
				}
			}
		}
		e.walk(el.List)
	}
}

// isNilTest reports whether the condition is "x != nil" (its else is not a
// catch-all).
func isNilTest(cond ast.Expr) bool {
	bin, ok := ast.Unparen(cond).(*ast.BinaryExpr)
	return ok && bin.Op == token.NEQ
}

// switchStmt reads the clauses of an expression switch: a tagless switch whose
// cases are error tests, or a switch on an error value with sentinel cases.
func (e *engine) switchStmt(s *ast.SwitchStmt) {
	var subjID *ast.Ident
	var subjObj types.Object
	if s.Tag != nil {
		if id, obj := e.subjectOf(s.Tag); obj != nil && errset.IsError(e.a.info.TypeOf(s.Tag)) {
			subjID, subjObj = id, obj
		}
	} else {
		for _, cc := range s.Body.List {
			for _, x := range cc.(*ast.CaseClause).List {
				if sub, _, ok := errset.ErrorTest(e.a.info, x); ok {
					if id, obj := e.subjectOf(sub); obj != nil {
						subjID, subjObj = id, obj
					}
				}
			}
			if subjObj != nil {
				break
			}
		}
	}
	if subjObj == nil {
		return
	}
	for _, s2 := range s.Body.List {
		cc := s2.(*ast.CaseClause)
		st, pos, has := e.blockStatus(cc.Body)
		if !has {
			continue
		}
		if len(cc.List) == 0 {
			e.addDef(subjID, subjObj, st, pos, "the default case at "+e.a.posStr(s.Pos()))
			continue
		}
		for _, x := range cc.List {
			var br errset.Branch
			ok := false
			if s.Tag == nil {
				_, br, ok = errset.ErrorTest(e.a.info, x)
			} else if k, isS := errset.SentinelKey(e.a.info, x); isS {
				br, ok = errset.Branch{Match: "sentinel", Key: k}, true
			}
			if ok {
				e.addRule(subjID, subjObj, br, st, pos, "switch case at "+e.a.posStr(s.Pos()), false)
			}
		}
	}
}

// finalize computes the statuses of the variants of every subject, and returns
// the 5xx findings of the handler (name and handlerPos label the handler).
func (e *engine) finalize(name, handlerPos string) []Finding {
	var out []Finding
	seen := map[string]bool{}
	report := func(v errset.Variant, st int, pos token.Pos, how string) {
		key := e.a.posStr(pos) + "|" + v.Key + "|" + v.Name
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, e.a.reachFinding(name, handlerPos, pos, v, st, how))
	}
	for _, obj := range e.order {
		if e.params[obj] {
			continue
		}
		// The default of a variable answers the variants that reach its own use
		// (the sink or the catch-all test). A variant rule answers the variants
		// that its branch matches at its own use. A variant at any other use is
		// not answered by either.
		if d, ok := e.defs[obj]; ok && d.id != nil {
			for _, v := range e.flow.Of(d.id) {
				if st, pos, how, ok := e.statusFor(obj, v); ok && st >= 500 {
					report(v, st, pos, how)
				}
			}
		}
		for _, r := range e.vrs {
			if r.subj != obj || r.id == nil {
				continue
			}
			for _, v := range e.flow.Of(r.id) {
				if !r.br.Matches(v) {
					continue
				}
				if st, pos, how, ok := e.statusFor(obj, v); ok && st >= 500 {
					report(v, st, pos, how)
				}
			}
		}
	}
	return out
}

// identOfObj returns the first identifier under node that uses obj, or nil.
func identOfObj(info *types.Info, node ast.Node, obj types.Object) *ast.Ident {
	var found *ast.Ident
	ast.Inspect(node, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && info.Uses[id] == obj {
			found = id
			return false
		}
		return true
	})
	return found
}

// statusFor returns the status of variant v of a subject.
func (e *engine) statusFor(obj types.Object, v errset.Variant) (int, token.Pos, string, bool) {
	for _, r := range e.vrs {
		if r.subj == obj && r.br.Matches(v) {
			return r.status, r.pos, r.how, r.status != 0
		}
	}
	if d, ok := e.defs[obj]; ok {
		return d.status, d.pos, d.how, d.status != 0
	}
	return 0, token.NoPos, "", false
}
