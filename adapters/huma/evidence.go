package huma

// Mapping evidence. A finding is "high" when the variant it reports is mapped to
// a non-5xx status somewhere else in the project. The mappings come from three
// places, all read by this file:
//
//  1. a condition that tests a variant and whose body returns a status-carrying
//     constructor below 500:
//     if errors.Is(err, service.ErrDuplicate) { return nil, huma.Error400BadRequest(…) }
//     if errors.As(err, &ve)                  { return huma.Error400BadRequest(…) }
//     if _, ok := errors.AsType[*T](err); ok { return huma.Error400BadRequest(…) }
//     if service.IsValidationError(err)       { return huma.Error400BadRequest(…) }
//     (the last form is a predicate: see PredicateFact)
//  2. the branches of a func(error) error (errset.Result.Mappers), such as palletError;
//  3. nothing else: a status that a variant reaches through a call is not a mapping.
//
// The status of a return is the status-function rule of the stub files: a call to
// huma.Error404NotFound is 404, huma.NewError(409, …) is 409 (see carried.go in errset).

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
)

// PredicateFact is exported on a func(error) bool whose body tests one concrete
// error type in one of the straight-line forms of errors.As (errorsAsShape) or
// errors.AsType (asTypeShape): service.IsValidationError tests validationError.
type PredicateFact struct {
	Key string // the tested type's key, pkgpath.Name
}

// AFact marks PredicateFact as an analysis fact.
func (*PredicateFact) AFact() {}

func (f *PredicateFact) String() string { return "huma-predicate: " + f.Key }

// statusFuncRules are the status-function rules of the stub files, by pkgpath.Name.
type statusFuncRules map[string]stubs.StatusValue

// loadStatusFuncRules reads the status rules that name a function or variable.
func loadStatusFuncRules(all []stubs.Rule) statusFuncRules {
	out := statusFuncRules{}
	for _, r := range all {
		if r.Verb != stubs.VerbStatus || r.Target.Name == "" {
			continue
		}
		if v, err := stubs.ParseStatusValue(r.Value); err == nil {
			out[r.Target.Pkg+"."+r.Target.Name] = v
		}
	}
	return out
}

// codeOf is the constant status of a call that a status-function rule names,
// or 0 when the call is not one or its status is not constant.
func (rules statusFuncRules) codeOf(info *types.Info, call *ast.CallExpr) int {
	var obj types.Object
	switch f := unparenExpr(call.Fun).(type) {
	case *ast.Ident:
		obj = info.Uses[f]
	case *ast.SelectorExpr:
		obj = info.Uses[f.Sel]
	}
	if obj == nil || obj.Pkg() == nil {
		return 0
	}
	sv, ok := rules[obj.Pkg().Path()+"."+obj.Name()]
	if !ok {
		return 0
	}
	if sv.ArgIdx < 0 {
		return sv.Code
	}
	if sv.ArgIdx >= len(call.Args) {
		return 0
	}
	tv, ok := info.Types[call.Args[sv.ArgIdx]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0
	}
	v, exact := constant.Int64Val(tv.Value)
	if !exact || v < 100 || v > 599 {
		return 0
	}
	return int(v)
}

// exportPredicates exports a PredicateFact on every func(error) bool of the package
// whose body is a straight-line test of one concrete error type, in the forms of
// errors.As (errorsAsShape) or errors.AsType (asTypeShape). These are the forms in
// which the SSA engine exports its test (errset's predicateFact), so both agree on
// which predicates test a type. A predicate with a constant return before its test
// exports nothing.
func exportPredicates(pass *analysis.Pass) map[*types.Func]string {
	out := map[*types.Func]string{}
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			obj, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func)
			if !ok || !isErrorPredicate(obj.Type().(*types.Signature)) {
				continue
			}
			k, ok := errorsAsShape(pass.TypesInfo, fd)
			if !ok {
				k, ok = asTypeShape(pass.TypesInfo, fd)
			}
			if ok {
				out[obj] = k
				pass.ExportObjectFact(obj, &PredicateFact{Key: k})
			}
		}
	}
	return out
}

// errorsAsShape reads the errors.As predicates in the straight-line forms that the
// SSA engine exports (errset's predicateFact), with p the parameter. Declarations of
// the target variables may come first. The rest of the body is one of:
//
//	return errors.As(p, &t)
//	if errors.As(p, &t) { return true }; return false        (or: … else { return false })
//	if !errors.As(p, &t) { return false }; return true
//
// Any other statement, such as an early return, makes the predicate a different
// test, so it exports nothing. It returns the key of t.
func errorsAsShape(info *types.Info, fd *ast.FuncDecl) (string, bool) {
	param := predicateParam(info, fd)
	list := fd.Body.List
	for len(list) > 0 {
		if _, isDecl := list[0].(*ast.DeclStmt); !isDecl {
			break
		}
		list = list[1:]
	}
	if param == nil || len(list) == 0 {
		return "", false
	}
	if r, ok := list[0].(*ast.ReturnStmt); ok {
		if len(list) != 1 || len(r.Results) != 1 {
			return "", false
		}
		return errorsAsTest(info, r.Results[0], param)
	}
	ifs, ok := list[0].(*ast.IfStmt)
	if !ok || ifs.Init != nil {
		return "", false
	}
	cond, negated := unparenExpr(ifs.Cond), false
	if u, isNot := cond.(*ast.UnaryExpr); isNot && u.Op == token.NOT {
		cond, negated = unparenExpr(u.X), true
	}
	key, ok := errorsAsTest(info, cond, param)
	if !ok || !onlyReturn(info, ifs.Body.List, !negated) {
		return "", false
	}
	if ifs.Else != nil {
		els, isBlock := ifs.Else.(*ast.BlockStmt)
		if !isBlock || negated || len(list) != 1 || !onlyReturn(info, els.List, false) {
			return "", false
		}
		return key, true
	}
	if len(list) != 2 || !onlyReturn(info, list[1:], negated) {
		return "", false
	}
	return key, true
}

// errorsAsTest reads errors.As(p, &t) on the parameter p and returns the key of t.
func errorsAsTest(info *types.Info, e ast.Expr, param types.Object) (string, bool) {
	call, ok := unparenExpr(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 2 || !isIdentOf(info, call.Args[0], param) {
		return "", false
	}
	return asTypeKey(info, call)
}

// onlyReturn reports whether stmts are the single statement `return v` for the
// constant bool v.
func onlyReturn(info *types.Info, stmts []ast.Stmt, want bool) bool {
	return len(stmts) == 1 && isReturnConst(info, stmts[0], want)
}

// predicateParam returns the object of the single named parameter of fd, or nil.
func predicateParam(info *types.Info, fd *ast.FuncDecl) types.Object {
	params := fd.Type.Params.List
	if len(params) != 1 || len(params[0].Names) != 1 {
		return nil
	}
	return info.Defs[params[0].Names[0]]
}

// asTypeShape reads the errors.AsType predicates that the SSA engine also exports
// (errset's TestFact), in their straight-line forms, with p the parameter. The
// test is either the assignment `_, ok := errors.AsType[T](p)` followed by one of
// the two results below, or the if-init form `if _, ok := errors.AsType[T](p); ok`
// followed by the same result:
//
//	return ok
//	if ok { return true }; return false        (or: if ok { return true } else { return false })
//
// Any other statement, such as an early return, makes the predicate a different
// test, so it exports nothing. It returns the key of T.
func asTypeShape(info *types.Info, fd *ast.FuncDecl) (string, bool) {
	param := predicateParam(info, fd)
	list := fd.Body.List
	if param == nil || len(list) == 0 {
		return "", false
	}
	switch s := list[0].(type) {
	case *ast.AssignStmt:
		key, flag, ok := asTypeAssign(info, s, param)
		if !ok {
			return "", false
		}
		tail := list[1:]
		if (len(tail) == 1 && isReturnOf(info, tail[0], flag)) || trueThenFalse(info, tail, flag) {
			return key, true
		}
	case *ast.IfStmt:
		if s.Init == nil {
			return "", false
		}
		key, flag, ok := asTypeAssign(info, s.Init, param)
		if ok && trueThenFalse(info, list, flag) {
			return key, true
		}
	}
	return "", false
}

// trueThenFalse reports whether stmts are `if flag { return true }` followed by
// `return false`, or that if with an else that returns false. The condition must
// denote flag itself, so a test of a shadowing ok does not count.
func trueThenFalse(info *types.Info, stmts []ast.Stmt, flag types.Object) bool {
	if len(stmts) == 0 {
		return false
	}
	ifs, ok := stmts[0].(*ast.IfStmt)
	if !ok || !isIdentOf(info, ifs.Cond, flag) || len(ifs.Body.List) != 1 || !isReturnConst(info, ifs.Body.List[0], true) {
		return false
	}
	if ifs.Else == nil {
		return len(stmts) == 2 && isReturnConst(info, stmts[1], false)
	}
	els, ok := ifs.Else.(*ast.BlockStmt)
	return ok && len(stmts) == 1 && len(els.List) == 1 && isReturnConst(info, els.List[0], false)
}

// asTypeAssign reads `_, ok := errors.AsType[T](p)` (or with =), where p is the
// parameter param. It returns the key of T, which must be a named concrete error
// type (as for errors.As), and the object of ok.
func asTypeAssign(info *types.Info, s ast.Stmt, param types.Object) (string, types.Object, bool) {
	as, ok := s.(*ast.AssignStmt)
	if !ok || (as.Tok != token.DEFINE && as.Tok != token.ASSIGN) || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
		return "", nil, false
	}
	call, ok := unparenExpr(as.Rhs[0]).(*ast.CallExpr)
	if !ok {
		return "", nil, false
	}
	subject, targ, ok := errset.AsTypeCall(info, call)
	if !ok || !isIdentOf(info, subject, param) {
		return "", nil, false
	}
	flag, ok := as.Lhs[1].(*ast.Ident)
	if !ok || flag.Name == "_" {
		return "", nil, false
	}
	obj := info.ObjectOf(flag)
	n := namedErrorTypeOf(targ)
	if obj == nil || n == nil {
		return "", nil, false
	}
	return objKeyOf(n.Obj()), obj, true
}

// isIdentOf reports whether e is an identifier that denotes obj.
func isIdentOf(info *types.Info, e ast.Expr, obj types.Object) bool {
	id, ok := unparenExpr(e).(*ast.Ident)
	return ok && obj != nil && info.Uses[id] == obj
}

// isReturnOf reports whether s is `return obj`.
func isReturnOf(info *types.Info, s ast.Stmt, obj types.Object) bool {
	r, ok := s.(*ast.ReturnStmt)
	return ok && len(r.Results) == 1 && isIdentOf(info, r.Results[0], obj)
}

// isReturnConst reports whether s is `return v` for the constant bool v.
func isReturnConst(info *types.Info, s ast.Stmt, want bool) bool {
	r, ok := s.(*ast.ReturnStmt)
	if !ok || len(r.Results) != 1 {
		return false
	}
	tv, ok := info.Types[r.Results[0]]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Bool && constant.BoolVal(tv.Value) == want
}

// isErrorPredicate reports whether sig is func(error) bool.
func isErrorPredicate(sig *types.Signature) bool {
	return sig.Params().Len() == 1 && sig.Results().Len() == 1 &&
		types.Identical(sig.Params().At(0).Type(), types.Universe.Lookup("error").Type()) &&
		types.Identical(sig.Results().At(0).Type(), types.Typ[types.Bool])
}

// asTypeKey returns the key of the concrete error type that an errors.As call
// tests (errors.As(err, &T) with T a named non-interface error type). It reads
// errors.As only: errors.AsType is read by asTypeShape (predicates) and ifKeys
// (the if-init form).
func asTypeKey(info *types.Info, call *ast.CallExpr) (string, bool) {
	fn := typeutil.StaticCallee(info, call)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != "errors" || fn.Name() != "As" || len(call.Args) != 2 {
		return "", false
	}
	u, ok := unparenExpr(call.Args[1]).(*ast.UnaryExpr)
	if !ok || u.Op != token.AND {
		return "", false
	}
	n := namedErrorTypeOf(info.TypeOf(u.X))
	if n == nil {
		return "", false
	}
	return objKeyOf(n.Obj()), true
}

// sentinelKeyOfExpr returns the key of a package-level error variable named by e.
func sentinelKeyOfExpr(info *types.Info, e ast.Expr) (string, bool) {
	var obj types.Object
	switch x := unparenExpr(e).(type) {
	case *ast.Ident:
		obj = info.Uses[x]
	case *ast.SelectorExpr:
		obj = info.Uses[x.Sel]
	}
	v, ok := obj.(*types.Var)
	if !ok || v.Pkg() == nil || v.IsField() || v.Parent() != v.Pkg().Scope() {
		return "", false
	}
	if !types.Implements(v.Type(), types.Universe.Lookup("error").Type().Underlying().(*types.Interface)) {
		return "", false
	}
	return objKeyOf(v), true
}

// namedErrorTypeOf returns T for T or *T when T is a named non-interface type
// that implements error (through its pointer, as errors.As targets do).
func namedErrorTypeOf(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || types.IsInterface(n) {
		return nil
	}
	if !types.Implements(types.NewPointer(n), types.Universe.Lookup("error").Type().Underlying().(*types.Interface)) {
		return nil
	}
	return n
}

func objKeyOf(o types.Object) string {
	if o.Pkg() == nil {
		return o.Name()
	}
	return o.Pkg().Path() + "." + o.Name()
}

// conditionKeys are the variant keys that a condition tests: errors.Is with a
// sentinel, errors.As with a concrete type, an == with a sentinel, and a call to
// a predicate that tests one type.
func conditionKeys(pass *analysis.Pass, local map[*types.Func]string, cond ast.Expr) []string {
	info := pass.TypesInfo
	switch x := unparenExpr(cond).(type) {
	case *ast.CallExpr:
		if k, ok := asTypeKey(info, x); ok {
			return []string{k}
		}
		fn := typeutil.StaticCallee(info, x)
		if fn == nil {
			return nil
		}
		if fn.Pkg() != nil && fn.Pkg().Path() == "errors" && fn.Name() == "Is" && len(x.Args) == 2 {
			if k, ok := sentinelKeyOfExpr(info, x.Args[1]); ok {
				return []string{k}
			}
			return nil
		}
		if k, ok := local[fn.Origin()]; ok {
			return []string{k}
		}
		var pf PredicateFact
		if fn.Pkg() != pass.Pkg && pass.ImportObjectFact(fn.Origin(), &pf) {
			return []string{pf.Key}
		}
	case *ast.BinaryExpr:
		if x.Op == token.EQL {
			if k, ok := sentinelKeyOfExpr(info, x.Y); ok {
				return []string{k}
			}
			if k, ok := sentinelKeyOfExpr(info, x.X); ok {
				return []string{k}
			}
		}
	}
	return nil
}

// ifKeys are the variant keys that an if statement tests: its condition, as
// conditionKeys reads it, or the errors.AsType form declared in its init statement.
func ifKeys(pass *analysis.Pass, local map[*types.Func]string, s *ast.IfStmt) []string {
	if keys := conditionKeys(pass, local, s.Cond); keys != nil {
		return keys
	}
	if _, b, ok := errset.IfErrorTest(pass.TypesInfo, s); ok {
		if k, isType := b.TypeKey(); isType {
			return []string{k}
		}
	}
	return nil
}

// bodyStatus is the one status below 500 that the returns of a body carry, or 0
// when they carry none or several.
func bodyStatus(rules statusFuncRules, info *types.Info, body []ast.Stmt) int {
	code := 0
	conflict := false
	for _, s := range body {
		ast.Inspect(s, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			r, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			for _, res := range r.Results {
				call, ok := unparenExpr(res).(*ast.CallExpr)
				if !ok {
					continue
				}
				c := rules.codeOf(info, call)
				if c < 100 || c >= 500 {
					continue
				}
				if code != 0 && code != c {
					conflict = true
				}
				code = c
			}
			return true
		})
	}
	if conflict {
		return 0
	}
	return code
}

// condMappings returns the mappings of every condition of the package: a
// condition that tests a variant whose body returns a status below 500.
func condMappings(pass *analysis.Pass, rules statusFuncRules, local map[*types.Func]string) []Mapping {
	var out []Mapping
	add := func(keys []string, body []ast.Stmt, cond ast.Node, where string) {
		if len(keys) == 0 {
			return
		}
		code := bodyStatus(rules, pass.TypesInfo, body)
		if code == 0 {
			return
		}
		for _, k := range keys {
			out = append(out, Mapping{Key: k, Status: code, Mapper: where, Pos: pass.Fset.Position(cond.Pos()).String()})
		}
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.IfStmt:
				add(ifKeys(pass, local, x), x.Body.List, x, enclosingName(f, x.Pos()))
			case *ast.SwitchStmt:
				if x.Tag != nil {
					return true
				}
				for _, s := range x.Body.List {
					cc := s.(*ast.CaseClause)
					for _, e := range cc.List {
						add(conditionKeys(pass, local, e), cc.Body, e, enclosingName(f, e.Pos()))
					}
				}
			}
			return true
		})
	}
	return out
}

// enclosingName names the function that contains pos: the declaration's name, or
// "func literal (file:line)" for a literal that is not in a declaration.
func enclosingName(f *ast.File, pos token.Pos) string {
	name := "package scope"
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil || pos < fd.Pos() || pos > fd.End() {
			continue
		}
		name = fd.Name.Name
	}
	return name
}

// mappingEvidence collects the mappings of a package: the conditions above and the
// branches of every func(error) error (errset.Result.Mappers).
func mappingEvidence(pass *analysis.Pass, res errset.Result, rules statusFuncRules) []Mapping {
	local := exportPredicates(pass)
	out := condMappings(pass, rules, local)
	for fn, fact := range res.Mappers {
		for _, m := range errset.MappingsOf(fact) {
			out = append(out, Mapping{Key: m.Key, Status: m.Status, Mapper: fn.Name(), Pos: pass.Fset.Position(fn.Pos()).String()})
		}
	}
	return out
}
