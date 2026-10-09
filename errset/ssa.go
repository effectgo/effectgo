package errset

// The SSA engine (-errset.engine=ssa). Each package is lowered to go/ssa in
// the pass (see buildSSA), so that a local error variable is the set of
// its SSA values, not the union of every assignment to it. The per-function
// analysis is in ssaflow.go; this file builds the package and the unit state
// that a declared function and its func literals share.

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Engine names of the -errset.engine flag.
const (
	engineAST = "ast"
	engineSSA = "ssa"
)

// buildSSA lowers the analyzed package to SSA and records the AST calls by
// their opening parenthesis, which is the position of an ssa.Call. It reports
// false when the lowering panics; the caller then keeps the AST engine for
// every unit. Only the analyzed package is built: its direct imports are
// created as synthetic packages (members without bodies), so references to
// their functions and variables resolve, and nothing else is lowered. The
// program is dropped when the pass returns.
func (p *pkgState) buildSSA() (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			p.ssaProg = nil
			ok = false
		}
	}()
	prog := ssa.NewProgram(p.pass.Fset, ssa.BareInits)
	for _, imp := range p.pass.Pkg.Imports() {
		prog.CreatePackage(imp, nil, nil, true)
	}
	sp := prog.CreatePackage(p.pass.Pkg, p.pass.Files, p.pass.TypesInfo, true)
	sp.Build()
	p.ssaProg = prog
	p.callAt = map[token.Pos]*ast.CallExpr{}
	for _, f := range p.pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				p.callAt[c.Lparen] = c
			}
			return true
		})
	}
	return true
}

// ssaFuncOf returns the SSA function of a declared unit, or nil when SSA has
// no body for it (a generic function: ssa lowers only instances).
func (p *pkgState) ssaFuncOf(u *fnUnit) *ssa.Function {
	if p.ssaProg == nil {
		return nil
	}
	fn := p.ssaProg.FuncValue(u.obj)
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}
	return fn
}

// ssaUnit is the state that a declared function and the func literals inside
// it share during one evaluation: the literals' return sets and the union of
// the values stored into each captured variable (see root).
type ssaUnit struct {
	p     *pkgState
	top   *fnUnit
	owner string
	fns   []*ssaFn // the declared function first, then its literals, outer before inner
	// byFn finds the state of an SSA function of this unit.
	byFn map[*ssa.Function]*ssaFn
	// makeClosureOf is the MakeClosure that creates each literal.
	makeClosureOf map[*ssa.Function]*ssa.MakeClosure
	// freeOwner maps each free variable to its literal and its index in the
	// literal's FreeVars (the index of the MakeClosure binding).
	freeOwner map[*ssa.FreeVar]freeRef
	// ownParams maps the parameters of the declared function (not the
	// receiver, not the literals') to their 0-based position, as ownParamIndex.
	ownParams map[*ssa.Parameter]int
	// allocs is the union of the error sets stored into each captured or
	// address-taken variable (an *ssa.Alloc). A load of it yields this set.
	allocs map[*ssa.Alloc]errSet
	// lits is the error set of each literal with an error result.
	lits map[*ssa.Function]errSet
}

type freeRef struct {
	fn    *ssa.Function
	index int
}

// newSSAUnit prepares the state of the unit whose SSA function is top.
func (p *pkgState) newSSAUnit(u *fnUnit, top *ssa.Function) *ssaUnit {
	un := &ssaUnit{
		p: p, top: u, owner: funcName(u.obj),
		byFn:          map[*ssa.Function]*ssaFn{},
		makeClosureOf: map[*ssa.Function]*ssa.MakeClosure{},
		freeOwner:     map[*ssa.FreeVar]freeRef{},
		ownParams:     map[*ssa.Parameter]int{},
		allocs:        map[*ssa.Alloc]errSet{},
		lits:          map[*ssa.Function]errSet{},
	}
	var walk func(fn *ssa.Function)
	walk = func(fn *ssa.Function) {
		f := newSSAFn(un, fn)
		un.fns = append(un.fns, f)
		un.byFn[fn] = f
		for _, anon := range fn.AnonFuncs {
			walk(anon)
		}
	}
	walk(top)
	for _, f := range un.fns {
		for i, fv := range f.fn.FreeVars {
			un.freeOwner[fv] = freeRef{fn: f.fn, index: i}
		}
		for _, b := range f.fn.Blocks {
			for _, in := range b.Instrs {
				if mc, ok := in.(*ssa.MakeClosure); ok {
					if anon, ok := mc.Fn.(*ssa.Function); ok {
						un.makeClosureOf[anon] = mc
					}
				}
			}
		}
	}
	recv := 0
	if top.Signature.Recv() != nil {
		recv = 1
	}
	for i, prm := range top.Params {
		if i < recv {
			continue
		}
		un.ownParams[prm] = i - recv
	}
	return un
}

// root returns the variable that an address refers to: an *ssa.Alloc, or,
// for a free variable of a literal, the Alloc of the declared function that
// the literal captures (through its MakeClosure). It returns nil when the
// address is not a variable of this unit.
func (u *ssaUnit) root(addr ssa.Value) *ssa.Alloc {
	switch a := addr.(type) {
	case *ssa.Alloc:
		return a
	case *ssa.FreeVar:
		ref, ok := u.freeOwner[a]
		if !ok {
			return nil
		}
		mc := u.makeClosureOf[ref.fn]
		if mc == nil || ref.index >= len(mc.Bindings) {
			return nil
		}
		return u.root(mc.Bindings[ref.index])
	}
	return nil
}

// litSet is the error set of a literal of this unit (empty until computed).
func (u *ssaUnit) litSet(fn *ssa.Function) errSet {
	if s, ok := u.lits[fn]; ok {
		return s
	}
	return errSet{}
}

// freshVariant is the fresh error created by the declared function owner
// (see freshSet in errset.go). It is shared by both engines.
func (p *pkgState) freshVariant(owner string) Variant {
	pkg := p.pass.Pkg
	return Variant{
		Key:   "new:" + pkg.Path() + "." + owner,
		Name:  freshPrefix + pkg.Name() + "." + owner,
		Kind:  KindFresh,
		Pkg:   pkg.Path(),
		Ident: lastComponent(owner),
	}
}

// callLabel is the display of a call that has no static callee: its source
// text when the AST call is known, else the fallback.
func callLabel(call *ast.CallExpr, fallback string) string {
	if call == nil {
		return fallback
	}
	return types.ExprString(call.Fun)
}

// constString returns the value of a constant string operand.
func constString(v ssa.Value) (string, bool) {
	c, ok := v.(*ssa.Const)
	if !ok || c.Value == nil || c.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(c.Value), true
}

// variadicValues returns the elements of a variadic argument: the slice that
// SSA builds from an Alloc of [n]T with one IndexAddr store per element. A nil
// slice constant has no elements. ok is false for any other form (a slice
// passed with an ellipsis), so the caller can give an opaque set.
func variadicValues(v ssa.Value) (vals []ssa.Value, ok bool) {
	if c, isConst := v.(*ssa.Const); isConst && c.IsNil() {
		return nil, true
	}
	sl, isSlice := v.(*ssa.Slice)
	if !isSlice {
		return nil, false
	}
	alloc, isAlloc := sl.X.(*ssa.Alloc)
	if !isAlloc {
		return nil, false
	}
	arr, isArr := alloc.Type().(*types.Pointer).Elem().Underlying().(*types.Array)
	if !isArr {
		return nil, false
	}
	vals = make([]ssa.Value, arr.Len())
	filled := make([]bool, arr.Len())
	if alloc.Referrers() == nil {
		return nil, false
	}
	for _, r := range *alloc.Referrers() {
		ia, isIA := r.(*ssa.IndexAddr)
		if !isIA {
			continue
		}
		idx, isConst := ia.Index.(*ssa.Const)
		if !isConst || idx.Value == nil {
			return nil, false
		}
		i64, exact := constant.Int64Val(idx.Value)
		if !exact || i64 < 0 || int(i64) >= len(vals) {
			return nil, false
		}
		if ia.Referrers() == nil {
			return nil, false
		}
		for _, sr := range *ia.Referrers() {
			st, isStore := sr.(*ssa.Store)
			if !isStore || st.Addr != ia {
				continue
			}
			vals[i64] = st.Val
			filled[i64] = true
		}
	}
	for _, f := range filled {
		if !f {
			return nil, false
		}
	}
	return vals, true
}
