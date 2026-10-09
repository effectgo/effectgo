package errset

// Flow-sensitive error sets over SSA (the ssa engine). The set of an SSA
// value is computed from its definition: a call from the callee's set, a load
// of a sentinel from the sentinel, a Phi from its incoming edges, and so on.
// A value has one set for each definition, so an error variable that is
// reassigned is not the union of its assignments.
//
// Narrowing (guards such as errors.Is, == nil, errors.As) is a forward
// dataflow over the blocks. The environment of a block holds the sets of the
// values that a guard on the way to the block has narrowed; a value that is
// not in it keeps its own set. See narrow in guards.go.

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// maxRounds bounds the fixed point of one unit. The sets only grow and the
// environments are monotone, so the bound is a safety net: when it is hit the
// unit falls back to the AST engine.
const maxRounds = 200

// ssaFn is the dataflow state of one SSA function of a unit (the declared
// function or one of its literals).
type ssaFn struct {
	u      *ssaUnit
	fn     *ssa.Function
	errIdx int // index of the error result, or -1
	rpo    []*ssa.BasicBlock
	reach  map[*ssa.BasicBlock]bool
	// set is the set of each error-typed instruction, as defined by its
	// transfer function (the union over rounds).
	set map[ssa.Value]errSet
	// env is the environment at the entry of each block: the narrowed sets of
	// the values that every path to the block narrowed. A value defined in the
	// block itself is never in its entry environment.
	env  map[*ssa.BasicBlock]map[ssa.Value]errSet
	done map[*ssa.BasicBlock]bool
}

func newSSAFn(u *ssaUnit, fn *ssa.Function) *ssaFn {
	f := &ssaFn{
		u: u, fn: fn, errIdx: errIndex(fn.Signature),
		reach: map[*ssa.BasicBlock]bool{},
		set:   map[ssa.Value]errSet{},
		env:   map[*ssa.BasicBlock]map[ssa.Value]errSet{},
		done:  map[*ssa.BasicBlock]bool{},
	}
	if len(fn.Blocks) > 0 {
		f.rpo = reversePostorder(fn.Blocks[0])
		for _, b := range f.rpo {
			f.reach[b] = true
		}
	}
	return f
}

// reversePostorder lists the blocks reachable from entry in reverse postorder.
func reversePostorder(entry *ssa.BasicBlock) []*ssa.BasicBlock {
	seen := map[*ssa.BasicBlock]bool{}
	var post []*ssa.BasicBlock
	var dfs func(b *ssa.BasicBlock)
	dfs = func(b *ssa.BasicBlock) {
		seen[b] = true
		for _, s := range b.Succs {
			if !seen[s] {
				dfs(s)
			}
		}
		post = append(post, b)
	}
	dfs(entry)
	out := make([]*ssa.BasicBlock, len(post))
	for i, b := range post {
		out[len(post)-1-i] = b
	}
	return out
}

// defBlockOf returns the block that defines v, or nil for a value defined
// outside the blocks (a parameter, a constant, a global, a free variable).
func defBlockOf(v ssa.Value) *ssa.BasicBlock {
	if in, ok := v.(ssa.Instruction); ok {
		return in.Block()
	}
	return nil
}

// iterate evaluates every block once, in reverse postorder, and reports
// whether a set or an environment changed.
func (f *ssaFn) iterate() bool {
	changed := false
	for _, b := range f.rpo {
		var env map[ssa.Value]errSet
		if b == f.fn.Blocks[0] {
			env = map[ssa.Value]errSet{}
		} else {
			var ready bool
			env, ready = f.joinEnv(b)
			if !ready {
				continue
			}
		}
		for v := range env {
			if defBlockOf(v) == b {
				delete(env, v) // a value that b defines is never narrowed on entry
			}
		}
		if !f.done[b] || !sameEnv(f.env[b], env) {
			changed = true
		}
		f.env[b] = env
		f.done[b] = true
		for _, in := range b.Instrs {
			v, ok := in.(ssa.Value)
			if !ok || !carriesError(v.Type()) {
				continue
			}
			s := f.transfer(v, b)
			cur := f.set[v]
			if cur == nil {
				cur = errSet{}
				f.set[v] = cur
			}
			if cur.putAll(s) {
				changed = true
			}
		}
	}
	return changed
}

// joinEnv is the entry environment of b: the join of the edge environments of
// its evaluated predecessors. A value is narrowed at b only when every such
// edge narrows it; otherwise it keeps its own set (the top). ready is false
// when no predecessor has been evaluated yet.
func (f *ssaFn) joinEnv(b *ssa.BasicBlock) (map[ssa.Value]errSet, bool) {
	var edges []map[ssa.Value]errSet
	for _, p := range b.Preds {
		if !f.reach[p] || !f.done[p] {
			continue
		}
		edges = append(edges, f.edgeEnv(p, b))
	}
	if len(edges) == 0 {
		return nil, false
	}
	out := map[ssa.Value]errSet{}
	for k := range edges[0] {
		s := errSet{}
		all := true
		for _, e := range edges {
			x, ok := e[k]
			if !ok {
				all = false
				break
			}
			s.putAll(x)
		}
		if all {
			out[k] = s
		}
	}
	return out, true
}

// edgeEnv is the environment at the end of p on its edge to b: the entry
// environment of p without the values that p defines, then the narrowing of
// p's branch on that edge.
func (f *ssaFn) edgeEnv(p, b *ssa.BasicBlock) map[ssa.Value]errSet {
	e := map[ssa.Value]errSet{}
	for v, s := range f.env[p] {
		if defBlockOf(v) == p {
			continue
		}
		e[v] = s
	}
	f.narrowEdge(p, b, e)
	return e
}

// effective is the set of v at a point, given the environment env of that
// point: the entry environment of a block for a use in it, or the edge
// environment for the end of a predecessor (which narrows the values that
// the predecessor defines too). Without an entry, v keeps its own set.
func (f *ssaFn) effective(v ssa.Value, env map[ssa.Value]errSet) errSet {
	if c, ok := v.(*ssa.Const); ok && c.IsNil() {
		return errSet{}
	}
	base := f.defSet(v)
	if s, ok := env[v]; ok {
		return intersectSet(s, base)
	}
	return base
}

// intersectSet keeps the variants of a that b also has, with b's witness.
func intersectSet(a, b errSet) errSet {
	out := errSet{}
	for k, v := range b {
		if _, ok := a[k]; ok {
			out[k] = v
		}
	}
	return out
}

// defSet is the set that a value has by its definition (before narrowing).
func (f *ssaFn) defSet(v ssa.Value) errSet {
	switch x := v.(type) {
	case *ssa.Const:
		if x.IsNil() {
			return errSet{}
		}
		return setOf(opaqueVariant())
	case *ssa.Parameter:
		return setOf(opaqueVariant())
	}
	if s, ok := f.set[v]; ok {
		return s
	}
	return errSet{}
}

// litOf returns the literal that an SSA value denotes: a MakeClosure of a
// literal with free variables, or the literal itself when it has none (SSA
// then uses the *ssa.Function directly).
func litOf(v ssa.Value) (*ssa.Function, bool) {
	var fn *ssa.Function
	switch x := v.(type) {
	case *ssa.MakeClosure:
		fn, _ = x.Fn.(*ssa.Function)
	case *ssa.Function:
		fn = x
	}
	// A bound method value (x.M) is a MakeClosure of a synthetic wrapper with
	// no parent: it is not a literal, so it stays a dynamic call.
	return fn, fn != nil && fn.Parent() != nil
}

// capturedParam returns the index of the declared function's own parameter
// that the variable root holds, when every store into root stores that
// parameter (a func parameter that a literal captures: the caller's
// parameter composes, as in the AST engine).
func (u *ssaUnit) capturedParam(root *ssa.Alloc) (int, bool) {
	if root.Referrers() == nil {
		return 0, false
	}
	idx, found := 0, false
	for _, r := range *root.Referrers() {
		st, ok := r.(*ssa.Store)
		if !ok || st.Addr != root {
			continue
		}
		par, ok := st.Val.(*ssa.Parameter)
		if !ok {
			return 0, false
		}
		i, ok := u.ownParams[par]
		if !ok || (found && i != idx) {
			return 0, false
		}
		idx, found = i, true
	}
	return idx, found
}

// carriesError reports whether a value of type t can hold an error: an error
// type, or an interface type (an any that a variadic argument boxes an error
// into is one: fmt.Errorf's %w arguments have that type).
func carriesError(t types.Type) bool {
	return implementsError(t) || types.IsInterface(t)
}

// transfer is the set of an instruction value defined in b. A value of an
// interface type that is not an error (an any argument) only passes the sets
// through the conversions and the type assertions; a call or a Phi of such a
// type is opaque, since its results are not the errors of the callee.
func (f *ssaFn) transfer(v ssa.Value, b *ssa.BasicBlock) errSet {
	env := f.env[b]
	if !implementsError(v.Type()) {
		switch x := v.(type) {
		case *ssa.ChangeInterface:
			return f.effective(x.X, env)
		case *ssa.MakeInterface:
			if n := namedErrorType(x.X.Type()); n != nil {
				return setOf(f.u.p.typeVariant(n))
			}
		case *ssa.TypeAssert:
			return f.typeAssertSet(x.AssertedType)
		}
		return setOf(opaqueVariant())
	}
	switch x := v.(type) {
	case *ssa.Phi:
		out := errSet{}
		for i, e := range x.Edges {
			p := b.Preds[i]
			if !f.reach[p] || !f.done[p] {
				continue
			}
			out.putAll(f.effective(e, f.edgeEnv(p, b)))
		}
		return out
	case *ssa.Call:
		return f.callSet(&x.Call, x.Pos(), b, env)
	case *ssa.Extract:
		switch t := x.Tuple.(type) {
		case *ssa.Call:
			if x.Index == errIndex(t.Call.Signature()) {
				return f.callSet(&t.Call, t.Pos(), b, env)
			}
		case *ssa.TypeAssert:
			if x.Index == 0 {
				return f.typeAssertSet(t.AssertedType)
			}
		}
		return setOf(opaqueVariant())
	case *ssa.UnOp:
		if x.Op != token.MUL {
			return setOf(opaqueVariant())
		}
		if g, ok := x.X.(*ssa.Global); ok {
			if sv, ok := sentinelKey(g.Object()); ok {
				return setOf(Variant{Key: objKey(sv), Name: objDisplay(sv), Kind: KindSentinel, Methods: methodsOf(sv.Type())})
			}
			return setOf(opaqueVariant())
		}
		if root := f.u.root(x.X); root != nil {
			out := errSet{}
			out.putAll(f.u.allocs[root])
			return out
		}
		return setOf(opaqueVariant())
	case *ssa.MakeInterface:
		if n := namedErrorType(x.X.Type()); n != nil {
			return setOf(f.u.p.typeVariant(n))
		}
		return setOf(opaqueVariant())
	case *ssa.ChangeInterface:
		return f.effective(x.X, env)
	case *ssa.ChangeType:
		return f.effective(x.X, env)
	case *ssa.TypeAssert:
		return f.typeAssertSet(x.AssertedType)
	}
	return setOf(opaqueVariant())
}

// typeAssertSet is the set of a value asserted to type t: the variant of t
// when t is a concrete error type, else opaque.
func (f *ssaFn) typeAssertSet(t types.Type) errSet {
	if n := namedErrorType(t); n != nil {
		return setOf(f.u.p.typeVariant(n))
	}
	return setOf(opaqueVariant())
}

// returnSet is the union of the error results of the return statements of
// the function, each read in the environment of its block.
func (f *ssaFn) returnSet() errSet {
	out := errSet{}
	if f.errIdx < 0 {
		return out
	}
	for _, b := range f.rpo {
		if !f.done[b] || len(b.Instrs) == 0 {
			continue
		}
		r, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
		if !ok || len(r.Results) <= f.errIdx {
			continue
		}
		out.putAll(f.effective(r.Results[f.errIdx], f.env[b]))
	}
	return out
}

// sameEnv reports whether two environments hold the same keys with the same
// variant keys.
func sameEnv(a, b map[ssa.Value]errSet) bool {
	if len(a) != len(b) {
		return false
	}
	for k, sa := range a {
		sb, ok := b[k]
		if !ok || len(sa) != len(sb) {
			return false
		}
		for vk := range sa {
			if _, ok := sb[vk]; !ok {
				return false
			}
		}
	}
	return true
}

// solve runs the functions of the unit to a fixed point. Each round evaluates
// every function, then recomputes the union of the stores into each captured
// variable and the return set of each literal. It reports false when the
// round bound is hit.
func (u *ssaUnit) solve() bool {
	for round := 0; round < maxRounds; round++ {
		changed := false
		for _, f := range u.fns {
			if f.iterate() {
				changed = true
			}
		}
		for _, f := range u.fns {
			for _, b := range f.rpo {
				if !f.done[b] {
					continue
				}
				for _, in := range b.Instrs {
					st, ok := in.(*ssa.Store)
					if !ok || !implementsError(st.Val.Type()) {
						continue
					}
					root := u.root(st.Addr)
					if root == nil {
						continue
					}
					s := f.effective(st.Val, f.env[b])
					cur := u.allocs[root]
					if cur == nil {
						cur = errSet{}
						u.allocs[root] = cur
					}
					if cur.putAll(s) {
						changed = true
					}
				}
			}
		}
		for _, f := range u.fns[1:] {
			s := f.returnSet()
			cur := u.lits[f.fn]
			if cur == nil {
				cur = errSet{}
				u.lits[f.fn] = cur
			}
			if cur.putAll(s) {
				changed = true
			}
		}
		if !changed {
			return true
		}
	}
	return false
}

// callSet is the error set of a call made in block b, with the arguments
// read at the call. pos is the call's opening parenthesis (its AST call is
// found through pkgState.callAt).
func (f *ssaFn) callSet(cc *ssa.CallCommon, pos token.Pos, b *ssa.BasicBlock, env map[ssa.Value]errSet) errSet {
	p := f.u.p
	call := p.callAt[pos]
	if cc.IsInvoke() {
		m := cc.Method.Origin()
		if s, ok := p.stubSet(m); ok {
			return s.prefixed(stubLabel(m))
		}
		if allowed, ok := p.contractOf(m); ok {
			label := "(" + recvTypeName(m) + ")." + m.Name() + " [contract]"
			out := errSet{}
			for _, v := range allowed {
				v.Chain = []string{label}
				out.put(v)
			}
			return out
		}
		return setOf(opaqueVariant(callLabel(call, m.Name())))
	}
	if lf, ok := litOf(cc.Value); ok {
		return f.u.litSet(lf) // a literal called in place or through a value
	}
	if fn, ok := cc.Value.(*ssa.Function); ok {
		obj, ok := fn.Object().(*types.Func)
		if !ok {
			return setOf(opaqueVariant(callLabel(call, fn.Name())))
		}
		return f.staticCallSet(obj.Origin(), cc, call, b, env)
	}
	// Dynamic calls: the order of the AST engine's dynamicCallSet.
	if call != nil {
		if v, ok := p.carriedOfVarCall(call); ok {
			return setOf(v)
		}
		if s, label, ok := p.dynamicStubSet(p.pass.TypesInfo, call); ok {
			return s.prefixed(label)
		}
	}
	if par, ok := cc.Value.(*ssa.Parameter); ok {
		if i, ok := f.u.ownParams[par]; ok {
			return setOf(paramVariant(i))
		}
	}
	if un, ok := cc.Value.(*ssa.UnOp); ok && un.Op == token.MUL {
		if root := f.u.root(un.X); root != nil {
			if i, ok := f.u.capturedParam(root); ok {
				return setOf(paramVariant(i))
			}
		}
	}
	return setOf(opaqueVariant(callLabel(call, "?")))
}

// staticCallSet is the error set of a call to a declared function, as the
// AST engine's callSet, with the arguments read as SSA values.
func (f *ssaFn) staticCallSet(callee *types.Func, cc *ssa.CallCommon, call *ast.CallExpr, b *ssa.BasicBlock, env map[ssa.Value]errSet) errSet {
	p := f.u.p
	if v, ok := p.carriedOfCall(callee, call); ok {
		return setOf(v)
	}
	if s, ok := p.stubSet(callee); ok {
		return s.prefixed(stubLabel(callee))
	}
	name := funcName(callee)
	pkg := callee.Pkg()
	if pkg != nil {
		switch {
		case pkg.Path() == "fmt" && callee.Name() == "Errorf":
			return f.errorfSet(cc, b, env)
		case pkg.Path() == "errors" && callee.Name() == "Join":
			return f.joinSet(cc, b, env)
		case pkg.Path() == "errors" && callee.Name() == "New":
			return setOf(p.freshVariant(f.u.owner))
		}
	}
	if pkg == nil {
		return setOf(opaqueVariant(name))
	}
	if isStd(p.pass.Fset, pkg) {
		return setOf(opaqueVariant(pkg.Name() + "." + name))
	}
	if pkg == p.pass.Pkg {
		if w, ok := p.work[callee]; ok {
			return f.bind(w.errs, cc, callee, b, env, name)
		}
		return setOf(opaqueVariant(name))
	}
	var fact ErrFact
	if p.pass.ImportObjectFact(callee, &fact) {
		return f.bind(setOf(fact.Errors...), cc, callee, b, env, name)
	}
	return setOf(opaqueVariant(pkg.Name() + "." + name))
}

// bind substitutes the param:i variants of a callee's set with the sets of
// the call's arguments, and prefixes each witness with the callee name. The
// receiver is an SSA argument, so an explicit parameter i is argument i+1 of
// a method.
func (f *ssaFn) bind(s errSet, cc *ssa.CallCommon, callee *types.Func, b *ssa.BasicBlock, env map[ssa.Value]errSet, name string) errSet {
	sig := callee.Type().(*types.Signature)
	off := 0
	if sig.Recv() != nil {
		off = 1
	}
	out := errSet{}
	for _, v := range s {
		if v.Kind != KindParam {
			v.Chain = append([]string{name}, v.Chain...)
			out.put(v)
			continue
		}
		i := paramIndex(v)
		if i < 0 || (sig.Variadic() && i >= sig.Params().Len()-1) || i+off >= len(cc.Args) {
			out.put(opaqueVariant(name))
			continue
		}
		out.putAll(f.argSet(cc.Args[i+off], b, env).prefixed(name))
	}
	return out
}

// argSet is the error set that a func-typed argument can produce when called.
func (f *ssaFn) argSet(arg ssa.Value, b *ssa.BasicBlock, env map[ssa.Value]errSet) errSet {
	if lf, ok := litOf(arg); ok {
		return f.u.litSet(lf)
	}
	if fn, ok := arg.(*ssa.Function); ok {
		if obj, ok := fn.Object().(*types.Func); ok {
			return f.u.p.namedFuncSet(obj.Origin())
		}
	}
	if par, ok := arg.(*ssa.Parameter); ok {
		if i, ok := f.u.ownParams[par]; ok {
			return setOf(paramVariant(i)) // the caller's own parameter: composes
		}
	}
	return setOf(opaqueVariant())
}

// errorfSet: fmt.Errorf is the union of its %w arguments; a constant format
// with no %w creates a fresh error (see errorfSet in the AST engine).
func (f *ssaFn) errorfSet(cc *ssa.CallCommon, b *ssa.BasicBlock, env map[ssa.Value]errSet) errSet {
	fail := setOf(opaqueVariant("fmt.Errorf"))
	if len(cc.Args) < 2 {
		return fail
	}
	format, ok := constString(cc.Args[0])
	if !ok {
		return fail
	}
	idxs, ok := wrapVerbArgs(format)
	if !ok {
		return fail
	}
	if len(idxs) == 0 {
		return setOf(f.u.p.freshVariant(f.u.owner))
	}
	vals, ok := variadicValues(cc.Args[1])
	if !ok {
		return fail
	}
	out := errSet{}
	for _, i := range idxs {
		if i >= len(vals) {
			return fail
		}
		out.putAll(f.effective(vals[i], env))
	}
	return out
}

// joinSet: errors.Join is the union of its arguments.
func (f *ssaFn) joinSet(cc *ssa.CallCommon, b *ssa.BasicBlock, env map[ssa.Value]errSet) errSet {
	if len(cc.Args) != 1 {
		return setOf(opaqueVariant("errors.Join"))
	}
	vals, ok := variadicValues(cc.Args[0])
	if !ok {
		return setOf(opaqueVariant("errors.Join"))
	}
	out := errSet{}
	for _, v := range vals {
		out.putAll(f.effective(v, env))
	}
	return out
}

// handlerOf returns the handler set of the declared function: the union of
// the error sets of the literals it returns (see compute in the AST engine).
func (u *ssaUnit) handlerOf() (errSet, bool) {
	out := errSet{}
	has := false
	top := u.fns[0]
	for _, b := range top.rpo {
		if !top.done[b] || len(b.Instrs) == 0 {
			continue
		}
		r, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
		if !ok {
			continue
		}
		for _, res := range r.Results {
			lf, ok := litOf(res)
			if !ok || errIndex(lf.Signature) < 0 {
				continue
			}
			has = true
			out.putAll(u.litSet(lf))
		}
	}
	return out, has
}

// computeSSA evaluates one declared function with the SSA engine. ok is false
// when the function has no SSA body or its evaluation does not converge; the
// caller then uses the AST engine for it.
func (p *pkgState) computeSSA(u *fnUnit) (errs, handler errSet, hasHandler bool, ok bool) {
	fn := p.ssaFuncOf(u)
	if fn == nil {
		return nil, nil, false, false
	}
	un := p.newSSAUnit(u, fn)
	if !un.solve() {
		return nil, nil, false, false
	}
	errs = un.fns[0].returnSet()
	handler, hasHandler = un.handlerOf()
	for _, f := range un.fns[1:] {
		lit, isLit := f.fn.Syntax().(*ast.FuncLit)
		if !isLit || errIndex(f.fn.Signature) < 0 {
			continue
		}
		p.litSets[lit] = FuncErrors{Variants: p.visible(un.litSet(f.fn).list())}
	}
	return errs, handler, hasHandler, true
}
