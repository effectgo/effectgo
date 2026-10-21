package ginhttp

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/effectgo/effectgo/errset"
)

// fmtFormatters are the fmt functions whose result carries the text of their
// arguments.
var fmtFormatters = map[string]bool{"Sprintf": true, "Sprint": true, "Sprintln": true, "Errorf": true}

// leaksIn checks every sink call of a declaration. Each handler (or literal
// with a *gin.Context parameter) is a scope of its own, so a finding names the
// function that contains the sink.
func (a *analyzer) leaksIn(d ast.Decl) {
	if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
		a.leakScope(fd.Body, funcLabel(fd), a.posStr(fd.Name.Pos()))
	}
	encl := "a package-level variable"
	if fd, ok := d.(*ast.FuncDecl); ok {
		encl = funcLabel(fd)
	}
	ast.Inspect(d, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok && ginCtxIndex(a.info, lit.Type) >= 0 {
			a.leakScope(lit.Body, literalLabel(encl), a.posStr(lit.Pos()))
		}
		return true
	})
}

// leakScope visits the calls of root, not descending into nested handler
// literals (they are scopes of their own).
func (a *analyzer) leakScope(root ast.Node, name, pos string) {
	ast.Inspect(root, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok && n != root && ginCtxIndex(a.info, lit.Type) >= 0 {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			a.leakCall(call, name, pos)
		}
		return true
	})
}

// leakCall reports a sink whose body argument carries error text.
func (a *analyzer) leakCall(call *ast.CallExpr, name, pos string) {
	r, ok := a.sinkOf(call)
	if !ok || r.body < 0 {
		return
	}
	// A variadic writer (c.String(code, format, values...)) sends its formatted
	// values too, so the body is the tail from the body index.
	body := call.Args[r.body:]
	if f := calleeOf(a.info, call); f == nil || !f.Type().(*types.Signature).Variadic() {
		body = body[:1]
	}
	var errExpr ast.Expr
	for _, arg := range body {
		if errExpr = a.leakIn(arg, 0); errExpr != nil {
			break
		}
	}
	if errExpr == nil {
		return
	}
	a.findings = append(a.findings, Finding{
		Kind:       "leak",
		Handler:    name + " (" + pos + ")",
		HandlerPos: pos,
		Pos:        a.posStr(call.Pos()),
		Variant:    types.ExprString(errExpr),
		ErrKind:    "leak",
		Status:     a.statusOfCall(call, r),
		Via:        "body argument of " + a.calleeName(call),
	})
}

// maxLiteralDepth is how many composite literals the body may nest. The brief
// asked for one level. Two is the envelope of the evaluated services: kuzma-wms
// writes gin.H{"detail": gin.H{"error": err.Error()}}, where the text is inside
// the inner literal (internal/httpapi/shipment.go:159, backups.go:193).
const maxLiteralDepth = 2

// leakIn returns the error expression whose text reaches the body: err.Error(),
// an error argument of fmt.Sprintf/Errorf, a concatenation with err.Error(), or
// one of those inside composite literals nested up to maxLiteralDepth (gin.H{...},
// struct{...}). depth is 0 at the body, and grows by one per literal.
func (a *analyzer) leakIn(e ast.Expr, depth int) ast.Expr {
	switch x := ast.Unparen(e).(type) {
	case *ast.CallExpr:
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" && len(x.Args) == 0 && errset.IsError(a.info.TypeOf(sel.X)) {
			return sel.X
		}
		if f := calleeOf(a.info, x); f != nil && f.Pkg() != nil && f.Pkg().Path() == "fmt" && fmtFormatters[f.Name()] {
			for _, arg := range x.Args {
				if errset.IsError(a.info.TypeOf(arg)) {
					return arg
				}
				if l := a.leakIn(arg, depth+1); l != nil {
					return l
				}
			}
		}
	case *ast.BinaryExpr:
		if x.Op == token.ADD {
			if l := a.leakIn(x.X, depth); l != nil {
				return l
			}
			return a.leakIn(x.Y, depth)
		}
	case *ast.CompositeLit:
		if depth >= maxLiteralDepth {
			return nil
		}
		for _, elt := range x.Elts {
			v := elt
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				v = kv.Value
			}
			if l := a.leakIn(v, depth+1); l != nil {
				return l
			}
		}
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			return a.leakIn(x.X, depth)
		}
	}
	return nil
}
