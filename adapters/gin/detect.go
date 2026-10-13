package ginhttp

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/faintghost/effectgo/errset"
)

// sinkMapperOf detects a sink mapper: a function with a *gin.Context parameter
// and an error parameter, whose error branches answer with sinks. The
// branches are read by the rule engine on the error parameter. A mapper needs
// at least one variant branch; its default is the catch-all status.
func (a *analyzer) sinkMapperOf(fd *ast.FuncDecl, fn *types.Func) *sinkMapperFact {
	ctx := ginCtxIndex(a.info, fd.Type)
	errI := errIndexOf(a.info, fd.Type)
	if ctx < 0 || errI < 0 {
		return nil
	}
	params := paramObjs(a.info, fd.Type)
	if errI >= len(params) || params[errI] == nil {
		return nil
	}
	errObj := params[errI]
	pm := map[types.Object]bool{}
	for _, o := range params {
		if o != nil {
			pm[o] = true
		}
	}
	e := newEngine(a, errset.Flow{}, pm)
	e.walk(fd.Body.List)
	m := &sinkMapperFact{Ctx: ctx, Err: errI}
	for _, r := range e.vrs {
		if r.subj == errObj {
			m.Branches = append(m.Branches, mapBranch{Br: r.br, Status: r.status, Pos: a.posStr(r.pos)})
		}
	}
	if len(m.Branches) == 0 {
		return nil
	}
	if d, ok := e.defs[errObj]; ok {
		m.Default, m.HasDefault = d.status, true
	}
	return m
}

// statusMapperOf detects a status mapper: func(error) (int, bool) whose error
// branches return a constant status with true. The status is the one the
// returns give; no branch is a default.
func (a *analyzer) statusMapperOf(fd *ast.FuncDecl) *statusMapperFact {
	fn, ok := a.info.Defs[fd.Name].(*types.Func)
	if !ok {
		return nil
	}
	sig := fn.Type().(*types.Signature)
	if sig.Params().Len() != 1 || !errset.IsError(sig.Params().At(0).Type()) || sig.Results().Len() != 2 {
		return nil
	}
	if !isBasicKind(sig.Results().At(0).Type(), types.Int) || !isBasicKind(sig.Results().At(1).Type(), types.Bool) {
		return nil
	}
	params := paramObjs(a.info, fd.Type)
	if len(params) != 1 || params[0] == nil {
		return nil
	}
	errObj := params[0]
	var branches []mapBranch
	var visit func(list []ast.Stmt)
	visit = func(list []ast.Stmt) {
		for _, s := range list {
			switch x := s.(type) {
			case *ast.IfStmt:
				for cur := x; cur != nil; {
					if sub, br, ok := errset.IfErrorTest(a.info, cur); ok && sameObj(a.info, sub, errObj) {
						if st, pos, ok := returnStatus(a.info, cur.Body.List); ok {
							branches = append(branches, mapBranch{Br: br, Status: st, Pos: a.posStr(pos)})
						}
					}
					next, _ := cur.Else.(*ast.IfStmt)
					cur = next
				}
			case *ast.SwitchStmt:
				for _, cc := range x.Body.List {
					clause := cc.(*ast.CaseClause)
					for _, ex := range clause.List {
						sub, br, ok := errset.ErrorTest(a.info, ex)
						if !ok || !sameObj(a.info, sub, errObj) {
							continue
						}
						if st, pos, ok := returnStatus(a.info, clause.Body); ok {
							branches = append(branches, mapBranch{Br: br, Status: st, Pos: a.posStr(pos)})
						}
					}
				}
			}
		}
	}
	visit(fd.Body.List)
	if len(branches) == 0 {
		return nil
	}
	return &statusMapperFact{Branches: branches}
}

// returnStatus reads the first direct "return N, true" of a block: N is a
// positive constant.
func returnStatus(info *types.Info, list []ast.Stmt) (int, token.Pos, bool) {
	for _, s := range list {
		r, ok := s.(*ast.ReturnStmt)
		if !ok || len(r.Results) != 2 {
			continue
		}
		st, ok := constInt(info, r.Results[0])
		if !ok || st <= 0 {
			continue
		}
		if b, ok := constBool(info, r.Results[1]); !ok || !b {
			continue
		}
		return st, r.Pos(), true
	}
	return 0, token.NoPos, false
}

// isBasicKind reports whether t's underlying type is the basic kind k.
func isBasicKind(t types.Type, k types.BasicKind) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == k
}

// factoryLit returns the position of the handler literal that a factory returns,
// when the function returns a gin handler (func(*gin.Context)) built by a literal.
func (a *analyzer) factoryLit(fd *ast.FuncDecl) string {
	fn, ok := a.info.Defs[fd.Name].(*types.Func)
	if !ok {
		return ""
	}
	sig := fn.Type().(*types.Signature)
	handler := false
	for i := 0; i < sig.Results().Len(); i++ {
		if s, ok := sig.Results().At(i).Type().Underlying().(*types.Signature); ok && s.Params().Len() == 1 && isGinContext(s.Params().At(0).Type()) {
			handler = true
		}
	}
	if !handler {
		return ""
	}
	lit := ""
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			for _, r := range x.Results {
				if fl, ok := ast.Unparen(r).(*ast.FuncLit); ok && lit == "" && ginCtxIndex(a.info, fl.Type) >= 0 {
					lit = a.posStr(fl.Pos())
				}
			}
		}
		return true
	})
	return lit
}
