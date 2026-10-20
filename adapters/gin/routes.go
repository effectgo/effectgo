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

// isGroupCall reads a group call without treating a dynamic prefix as empty.
func (a *analyzer) isGroupCall(call *ast.CallExpr) (groupDef, bool) {
	if keyOf(calleeOf(a.info, call)) != groupKey || len(call.Args) != 1 {
		return groupDef{}, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return groupDef{}, false
	}
	prefix, constant := constString(a.info, call.Args[0])
	g := groupDef{base: sel.X, prefix: prefix}
	if !constant {
		g.unresolved = "router group prefix is not a constant string"
	}
	return g, true
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
			group, ok := a.isGroupCall(call)
			if !ok {
				return true
			}
			if id, ok := as.Lhs[0].(*ast.Ident); ok {
				if obj := a.objOf(id); obj != nil {
					a.groups[obj] = group
				}
			}
			return true
		})
	}
}

// prefixOf keeps the legacy display prefix and separately reports unknown parts.
func (a *analyzer) prefixOf(e ast.Expr, depth int) (string, string) {
	if depth > 32 {
		return "", "router group prefix exceeds resolution depth"
	}
	var group groupDef
	var ok bool
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		group, ok = a.groups[a.objOf(x)]
	case *ast.CallExpr:
		group, ok = a.isGroupCall(x)
	}
	if ok {
		prefix, reason := a.prefixOf(group.base, depth+1)
		if reason == "" {
			reason = group.unresolved
		}
		return prefix + group.prefix, reason
	}
	typ := a.info.TypeOf(e)
	if p, ok := typ.(*types.Pointer); ok {
		typ = p.Elem()
	}
	if named, ok := types.Unalias(typ).(*types.Named); ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == ginPkg && named.Obj().Name() == "Engine" {
		return "", ""
	}
	return "", "router group prefix is not resolved"
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
		path, constant := constString(a.info, call.Args[0])
		reason := ""
		if constant {
			var prefix string
			prefix, reason = a.prefixOf(sel.X, 0)
			path = prefix + path
		} else {
			path = ""
			reason = "path is not a constant string"
		}
		a.res.Routes = append(a.res.Routes, Route{Method: method, Path: path, PathUnresolved: reason, HandlerPos: handler, Pos: a.posStr(call.Pos())})
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
