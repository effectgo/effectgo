package errset

// Read-only views of the errset machinery for other analyzers (the gin HTTP
// adapter). They reuse the flow sets and the errors.Is / errors.As / == readers
// of this package; nothing here changes how a set is computed.
//
// A Flow answers per use: Of gives the set of an expression at the point where
// it is evaluated. Under the SSA engine that is the narrowed set of the value
// there (see ssaflow.go); under the AST engine it is the flow-insensitive set
// of the variable.

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

// Flow is the error flow of one declared function, read after the analysis.
// Under the AST engine its sets are flow-insensitive: a local error variable
// holds the union of everything assigned to it anywhere in the declaration,
// nested literals included. Under the SSA engine (table is set) each use has
// the set of its value at that point, computed when the pass ended.
type Flow struct {
	p     *pkgState
	c     *flowCtx
	table *flowTable
}

// Of returns the visible variants of an expression of error type inside the
// declaration, at the point where it is evaluated. An expression that the SSA
// engine has no use for gives the opaque variant "?", except an identifier
// whose variable has uses (the union of them). A nil Flow gives nil.
func (f Flow) Of(e ast.Expr) []Variant {
	if f.table != nil {
		if e == nil {
			return nil
		}
		e = ast.Unparen(e)
		if vs, ok := f.table.byExpr[e]; ok {
			return vs
		}
		if id, ok := e.(*ast.Ident); ok {
			if obj := f.p.pass.TypesInfo.Uses[id]; obj != nil {
				if vs, ok := f.table.byObj[obj]; ok {
					return vs
				}
			}
		}
		return f.p.visible(setOf(opaqueVariant()).list())
	}
	if f.c == nil || e == nil {
		return nil
	}
	return f.p.visible(f.c.exprSet(e).list())
}

// ErrorTest reads a test of an error value against a sentinel or a type: an
// errors.Is(subject, S) call, an errors.As(subject, &T) call, or
// subject == S (either operand order). subject is the tested expression. ok is
// false when cond is not one of these shapes.
func ErrorTest(info *types.Info, cond ast.Expr) (subject ast.Expr, b Branch, ok bool) {
	switch x := unparen(cond).(type) {
	case *ast.CallExpr:
		f := typeutil.StaticCallee(info, x)
		if f == nil || f.Pkg() == nil || f.Pkg().Path() != "errors" || len(x.Args) != 2 {
			return nil, Branch{}, false
		}
		switch f.Name() {
		case "Is":
			if k, ok := sentinelKeyOf(info, x.Args[1]); ok {
				return x.Args[0], Branch{Match: matchSentinel, Key: k}, true
			}
		case "As":
			u, isAddr := unparen(x.Args[1]).(*ast.UnaryExpr)
			if !isAddr || u.Op != token.AND {
				return nil, Branch{}, false
			}
			tb, ok := (&branchBuilder{}).targetBranch(info.TypeOf(u.X))
			if !ok {
				return nil, Branch{}, false
			}
			return x.Args[0], tb, true
		}
	case *ast.BinaryExpr:
		if x.Op != token.EQL {
			return nil, Branch{}, false
		}
		if k, ok := sentinelKeyOf(info, x.Y); ok {
			return x.X, Branch{Match: matchSentinel, Key: k}, true
		}
		if k, ok := sentinelKeyOf(info, x.X); ok {
			return x.Y, Branch{Match: matchSentinel, Key: k}, true
		}
	}
	return nil, Branch{}, false
}

// AsTypeCall reads a call of errors.AsType: its error argument and the type
// argument E of the instance. The callee is recognised by its object, the
// standard library's generic function, not by how the call is spelled, so every
// instance (errors.AsType[*T], errors.AsType[T], …) is the same test. ok is false
// for any other call.
func AsTypeCall(info *types.Info, call *ast.CallExpr) (subject ast.Expr, targ types.Type, ok bool) {
	if len(call.Args) != 1 {
		return nil, nil, false
	}
	fn := typeutil.StaticCallee(info, call)
	if fn == nil {
		return nil, nil, false
	}
	fn = fn.Origin()
	if fn.Pkg() == nil || fn.Pkg().Path() != "errors" || fn.Name() != "AsType" {
		return nil, nil, false
	}
	tup, isTuple := info.TypeOf(call).(*types.Tuple)
	if !isTuple || tup.Len() != 2 {
		return nil, nil, false
	}
	return call.Args[0], tup.At(0).Type(), true
}

// asTypeInit reads the errors.AsType form of an if statement: the ok result is
// declared or assigned by the init statement and the condition tests it, as in
// if _, ok := errors.AsType[T](err); ok { … }. It returns the tested expression
// and T. Other shapes are not read: a negated test, an ok variable tested in a
// later statement, and a compound condition.
func asTypeInit(info *types.Info, init ast.Stmt, cond ast.Expr) (subject ast.Expr, targ types.Type, ok bool) {
	as, isAssign := init.(*ast.AssignStmt)
	if !isAssign || (as.Tok != token.DEFINE && as.Tok != token.ASSIGN) || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
		return nil, nil, false
	}
	flag, isIdent := as.Lhs[1].(*ast.Ident)
	if !isIdent {
		return nil, nil, false
	}
	test, isIdent := unparen(cond).(*ast.Ident)
	if !isIdent {
		return nil, nil, false
	}
	obj := info.ObjectOf(flag)
	if obj == nil || info.Uses[test] != obj {
		return nil, nil, false
	}
	call, isCall := unparen(as.Rhs[0]).(*ast.CallExpr)
	if !isCall {
		return nil, nil, false
	}
	return AsTypeCall(info, call)
}

// IfErrorTest reads the test of an if statement: its condition, as ErrorTest
// reads it, or the errors.AsType form in its init statement (asTypeInit). It
// returns the tested expression and the branch that the test selects.
func IfErrorTest(info *types.Info, s *ast.IfStmt) (ast.Expr, Branch, bool) {
	if sub, b, ok := ErrorTest(info, s.Cond); ok {
		return sub, b, true
	}
	sub, targ, ok := asTypeInit(info, s.Init, s.Cond)
	if !ok {
		return nil, Branch{}, false
	}
	b, ok := (&branchBuilder{}).targetBranch(targ)
	return sub, b, ok
}

// TypeKey is the key of the concrete error type that the branch selects, and
// whether it selects one (an interface or a sentinel test does not).
func (b Branch) TypeKey() (string, bool) {
	return b.Key, b.Match == matchType
}

// Matches reports whether the branch selects variant v.
func (b Branch) Matches(v Variant) bool { return b.matches(v) }

// IsError reports whether t implements the predeclared error interface.
func IsError(t types.Type) bool { return implementsError(t) }

// SentinelKey returns the pkgpath.Name key of a package-level error sentinel
// expression, and whether e is one.
func SentinelKey(info *types.Info, e ast.Expr) (string, bool) {
	return sentinelKeyOf(info, e)
}
