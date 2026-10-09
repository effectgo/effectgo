package effects

// Via items of stub rules (see internal/stubs/effects_value.go). A stdlib
// function that calls a method of one of its arguments performs whatever that
// method performs, and the stdlib body is not analyzed, so the stub says which
// method it calls on which argument. The effects of that method are evaluated
// here, at the call site, from the static type of the argument:
//
//   - an interface-typed argument (for example an io.Writer parameter): the
//     method is called exactly as the caller would call it, so its effects are
//     those of the interface contract, or unknown when there is none. A method
//     that the interface does not have is unknown too: the dynamic type may
//     have it.
//   - a concrete argument: the dynamic type is the argument's type, so the
//     method is looked up in its method set. Its effects are the stub, taxonomy,
//     body or fact of that method (*bytes.Buffer.Write has none; *os.File
//     methods are fs). A method that the type does not have is never called, so
//     it contributes nothing.
//   - os.Stdin, os.Stdout and os.Stderr: stdio, whatever the method.
//   - io.Discard: nothing, since its writes do nothing.
//
// The witness of an item is the stub's step, then the method's chain, so that
// ToWriter → fmt.Fprintf → io.Writer.Write names the path.

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"

	"github.com/faintghost/effectgo/internal/stubs"
)

// viaHit is an effect that a call site performs through a via item. w starts
// with the callee's step; the caller prepends its own.
type viaHit struct {
	eff string
	w   Witness
}

// hasVia reports whether fn has via items in its stub rule.
func (c *checker) hasVia(fn *types.Func) bool {
	return len(c.stubs.viaOf(targetKey(fn))) > 0
}

// viaHits returns the effects of the via items of a call site's callee. A
// callee without via items, or a call with no callee, has none.
func (c *checker) viaHits(site callSite) []viaHit {
	if site.callee == nil {
		return nil
	}
	vias := c.stubs.viaOf(targetKey(site.callee))
	if len(vias) == 0 {
		return nil
	}
	self := stepOf(site.callee)
	var out []viaHit
	for _, v := range vias {
		if v.Stored {
			out = append(out, c.storedHits(self, site.call, v.Method)...)
			continue
		}
		idx := v.ArgIdx + site.offset
		if idx >= len(site.call.Args) {
			out = append(out, viaHit{EffUnkn, Witness{self, Step{Name: fmt.Sprintf("missing argument %d", v.ArgIdx)}}})
			continue
		}
		out = append(out, c.argViaHits(self, site.call.Args[idx], v)...)
	}
	return out
}

// argViaHits evaluates one via item for the argument arg of a call whose
// callee is self.
func (c *checker) argViaHits(self Step, arg ast.Expr, v stubs.Via) []viaHit {
	info := c.pass.TypesInfo
	unknownAt := func() []viaHit {
		return []viaHit{{EffUnkn, Witness{self, Step{Name: types.ExprString(arg) + "." + v.Method}}}}
	}
	if sel, ok := ast.Unparen(arg).(*ast.SelectorExpr); ok && info.Selections[sel] == nil {
		if pv := pkgVar(info.Uses[sel.Sel]); pv != nil {
			switch pv.Pkg().Path() + "." + pv.Name() {
			case "os.Stdin", "os.Stdout", "os.Stderr":
				return []viaHit{{EffStdio, Witness{self, pkgStep(pv.Pkg(), pv.Name())}}}
			case "io.Discard":
				return nil
			}
		}
	}
	t := info.TypeOf(arg)
	if t == nil {
		return unknownAt()
	}
	if b, ok := t.(*types.Basic); ok && b.Kind() == types.UntypedNil {
		return unknownAt()
	}
	obj, _, _ := types.LookupFieldOrMethod(t, false, nil, v.Method)
	if obj == nil {
		if types.IsInterface(t) {
			return unknownAt() // the dynamic type of an interface may have the method
		}
		return nil // a concrete dynamic type without the method: never called
	}
	m, ok := obj.(*types.Func)
	if !ok {
		return unknownAt()
	}
	m = m.Origin()
	var out []viaHit
	effs := c.calleeEffects(m)
	for _, e := range sortedKeys(effs) {
		if _, ok := paramIndexOf(e); ok {
			// The method calls one of its own parameters; that argument is not
			// known here.
			out = append(out, viaHit{EffUnkn, Witness{self, stepOf(m)}})
			continue
		}
		w := Witness{self}
		w = append(w, effs[e]...)
		out = append(out, viaHit{e, w})
	}
	return out
}

// localFacts is what the analysis knows about the local variables of the
// package, for stored-value items. A local is trusted only when its value
// cannot change behind the analysis's back: it has exactly one assignment, from
// a call, and every use of it is a selector on it (a method call or a field),
// except Reset and Init, which re-point a stored writer or reader (bufio's Reset
// does). Any other use, such as passing the variable, returning it, storing it
// or taking its address, makes it untrusted.
type localFacts struct {
	defs map[*types.Var][]ast.Expr // the right-hand side of each assignment; nil when not one-to-one
	bad  map[*types.Var]bool       // used other than as the base of a selector
}

// isLocalVar reports whether v is a variable of a function (not a field and not
// a package-level variable).
func isLocalVar(v *types.Var) bool {
	return v != nil && !v.IsField() && v.Pkg() != nil && v.Parent() != v.Pkg().Scope()
}

// locals computes the localFacts of the package once.
func (c *checker) locals() *localFacts {
	if c.lf != nil {
		return c.lf
	}
	info := c.pass.TypesInfo
	lf := &localFacts{defs: map[*types.Var][]ast.Expr{}, bad: map[*types.Var]bool{}}
	c.lf = lf
	varOf := func(id *ast.Ident) *types.Var {
		if v, ok := info.Defs[id].(*types.Var); ok {
			return v
		}
		v, _ := info.Uses[id].(*types.Var)
		return v
	}
	allowed := map[*ast.Ident]bool{}
	for _, f := range c.pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				for i, l := range n.Lhs {
					id, ok := ast.Unparen(l).(*ast.Ident)
					if !ok {
						continue
					}
					allowed[id] = true
					if v := varOf(id); isLocalVar(v) {
						var rhs ast.Expr
						if len(n.Lhs) == len(n.Rhs) {
							rhs = n.Rhs[i]
						}
						lf.defs[v] = append(lf.defs[v], rhs)
					}
				}
			case *ast.ValueSpec:
				for i, id := range n.Names {
					allowed[id] = true
					if len(n.Values) == 0 {
						continue // a zero value, not an assignment
					}
					if v := varOf(id); isLocalVar(v) {
						var rhs ast.Expr
						if len(n.Values) == len(n.Names) {
							rhs = n.Values[i]
						}
						lf.defs[v] = append(lf.defs[v], rhs)
					}
				}
			case *ast.RangeStmt:
				for _, e := range []ast.Expr{n.Key, n.Value} {
					if id, ok := ast.Unparen(e).(*ast.Ident); ok && id != nil {
						allowed[id] = true
						if v := varOf(id); isLocalVar(v) {
							lf.defs[v] = append(lf.defs[v], nil)
						}
					}
				}
			case *ast.SelectorExpr:
				if id, ok := ast.Unparen(n.X).(*ast.Ident); ok && n.Sel.Name != "Reset" && n.Sel.Name != "Init" {
					allowed[id] = true
				}
			}
			return true
		})
	}
	for _, f := range c.pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && !allowed[id] {
				if v, ok := info.Uses[id].(*types.Var); ok && isLocalVar(v) {
					lf.bad[v] = true
				}
			}
			return true
		})
	}
	return lf
}

// storedArg returns the argument that the receiver of a method call stores, if
// the receiver is a call of a function with a stores item (json.NewEncoder(w)),
// or a trusted local variable assigned from such a call. ok is false otherwise.
func (c *checker) storedArg(call *ast.CallExpr) (arg ast.Expr, recv ast.Expr, ok bool) {
	info := c.pass.TypesInfo
	sel, isSel := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !isSel {
		return nil, nil, false
	}
	recv = ast.Unparen(sel.X)
	src := recv
	if id, isID := recv.(*ast.Ident); isID {
		v, isVar := info.Uses[id].(*types.Var)
		if !isVar {
			return nil, recv, false
		}
		lf := c.locals()
		if lf.bad[v] || len(lf.defs[v]) != 1 || lf.defs[v][0] == nil {
			return nil, recv, false
		}
		src = ast.Unparen(lf.defs[v][0])
	}
	ctor, isCall := src.(*ast.CallExpr)
	if !isCall {
		return nil, recv, false
	}
	fn, isFn := typeutil.Callee(info, ctor).(*types.Func)
	if !isFn {
		return nil, recv, false
	}
	stores := c.stubs.storesOf(targetKey(fn))
	if len(stores) == 0 || stores[0] >= len(ctor.Args) {
		return nil, recv, false
	}
	return ctor.Args[stores[0]], recv, true
}

// storedHits evaluates a via:stored.<method> item of a call: the method is
// called on the value that the receiver stores, from that value's static type.
// A receiver whose stored value is not known (a field, a parameter, an escaped
// variable) is unknown, never silence.
func (c *checker) storedHits(self Step, call *ast.CallExpr, method string) []viaHit {
	arg, recv, ok := c.storedArg(call)
	if !ok {
		return []viaHit{{EffUnkn, Witness{self, Step{Name: "stored value of " + types.ExprString(recv)}}}}
	}
	return c.argViaHits(self, arg, stubs.Via{Method: method})
}
