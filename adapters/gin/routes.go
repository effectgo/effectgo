package ginhttp

import (
	"go/ast"
	"go/types"
)

// routeMethods are the gin router methods that register a route. Engine has
// them through its embedded RouterGroup, so the static callee is the group's.
var routeMethods = map[string]string{
	"(*github.com/gin-gonic/gin.RouterGroup).GET":    "GET",
	"(*github.com/gin-gonic/gin.RouterGroup).POST":   "POST",
	"(*github.com/gin-gonic/gin.RouterGroup).PUT":    "PUT",
	"(*github.com/gin-gonic/gin.RouterGroup).PATCH":  "PATCH",
	"(*github.com/gin-gonic/gin.RouterGroup).DELETE": "DELETE",
}

const groupKey = "(*github.com/gin-gonic/gin.RouterGroup).Group"

// isGroupCall reports whether call is x.Group("const"), and returns the prefix
// and the receiver x.
func (a *analyzer) isGroupCall(call *ast.CallExpr) (string, ast.Expr, bool) {
	if keyOf(calleeOf(a.info, call)) != groupKey || len(call.Args) != 1 {
		return "", nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil, false
	}
	prefix, ok := constString(a.info, call.Args[0])
	if !ok {
		return "", nil, false
	}
	return prefix, sel.X, true
}

// collectGroups records the router group variables: g := base.Group("prefix").
func (a *analyzer) collectGroups() {
	for _, f := range a.pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			call, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			prefix, base, ok := a.isGroupCall(call)
			if !ok {
				return true
			}
			if id, ok := as.Lhs[0].(*ast.Ident); ok {
				if obj := a.objOf(id); obj != nil {
					a.groups[obj] = groupDef{base: base, prefix: prefix}
				}
			}
			return true
		})
	}
}

// prefixOf returns the constant path prefix of a router expression ("" for the
// engine or an unknown receiver).
func (a *analyzer) prefixOf(e ast.Expr, depth int) string {
	if depth > 32 {
		return ""
	}
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		if g, ok := a.groups[a.objOf(x)]; ok {
			return a.prefixOf(g.base, depth+1) + g.prefix
		}
	case *ast.CallExpr:
		if prefix, base, ok := a.isGroupCall(x); ok {
			return a.prefixOf(base, depth+1) + prefix
		}
	}
	return ""
}

// routesIn records the routes registered under a declaration whose handler
// resolves to a handler literal or a declared handler function.
func (a *analyzer) routesIn(d ast.Decl) {
	ast.Inspect(d, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		method, ok := routeMethods[keyOf(calleeOf(a.info, call))]
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		handler := a.handlerPos(call.Args[len(call.Args)-1])
		if handler == "" {
			return true
		}
		path, ok := constString(a.info, call.Args[0])
		if ok {
			path = a.prefixOf(sel.X, 0) + path
		} else {
			path = ""
		}
		a.res.Routes = append(a.res.Routes, Route{Method: method, Path: path, HandlerPos: handler, Pos: a.posStr(call.Pos())})
		return true
	})
}

// handlerPos returns the position that identifies a handler argument: a gin
// handler literal, a declared function, or the literal a factory call returns.
func (a *analyzer) handlerPos(e ast.Expr) string {
	switch x := ast.Unparen(e).(type) {
	case *ast.FuncLit:
		if ginCtxIndex(a.info, x.Type) >= 0 {
			return a.posStr(x.Pos())
		}
	case *ast.Ident:
		if fn, ok := a.info.Uses[x].(*types.Func); ok {
			return a.posStr(fn.Pos())
		}
	case *ast.SelectorExpr:
		if fn, ok := a.info.Uses[x.Sel].(*types.Func); ok {
			return a.posStr(fn.Pos())
		}
	case *ast.CallExpr:
		return a.factoryOf(calleeOf(a.info, x))
	}
	return ""
}
