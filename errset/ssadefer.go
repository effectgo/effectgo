package errset

// Named results and deferred closures (the ssa engine).
//
// A function with a named error result that a closure captures, or that has
// a defer, keeps the result in an *ssa.Alloc. A return stores its value into
// the alloc, runs the deferred calls, and returns the alloc's value. The
// variable's value at the return is therefore not the union of every store to
// it: it is the value stored by the return statement that runs, plus whatever
// a closure stores after that (a deferred closure runs at the return, and a
// deferred closure that reads the variable reads that value).
//
// The rules, per variable A declared by a function F:
//
//   - the set that F returns through A is the set stored by each of its
//     returns (the value at that return, narrowed as usual), plus closureWrites:
//     the values stored into A by any other function of the unit;
//   - a deferred literal (deferOnly) reads rets, that set, not the union of
//     every store; a literal that is not deferred reads the union, because it
//     can run anywhere in the body;
//   - when A's address reaches a call or a store (escaped), the return adds an
//     opaque error: the call may write A through the pointer;
//   - when the unit calls recover, a panic that is recovered returns A as it
//     was at the panic, so the return keeps the union of the stores.
//
// The AST engine keeps the union (it has no per-return value).

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// scanUses computes the static facts of the unit: which literals run only as
// deferred calls, which variables escape through their address, and whether a
// function of the unit recovers.
func (u *ssaUnit) scanUses() {
	for _, f := range u.fns[1:] {
		mc := u.makeClosureOf[f.fn]
		if mc == nil || mc.Referrers() == nil || len(*mc.Referrers()) == 0 {
			continue
		}
		only := true
		for _, r := range *mc.Referrers() {
			if _, ok := r.(*ssa.Defer); !ok {
				only = false
			}
		}
		u.deferOnly[f.fn] = only
	}
	for _, f := range u.fns {
		for _, b := range f.fn.Blocks {
			for _, in := range b.Instrs {
				if c, ok := in.(*ssa.Call); ok {
					if bi, ok := c.Call.Value.(*ssa.Builtin); ok && bi.Name() == "recover" {
						u.recovers = true
					}
				}
				u.markEscapes(in)
			}
		}
	}
}

// markEscapes marks the variables whose address an instruction uses in a way
// that is not a load, a store into the variable, a closure binding or a debug
// reference.
func (u *ssaUnit) markEscapes(in ssa.Instruction) {
	switch x := in.(type) {
	case *ssa.UnOp:
		if x.Op == token.MUL {
			return
		}
	case *ssa.MakeClosure, *ssa.DebugRef:
		return
	case *ssa.Store:
		// The address operand is stored into; a stored address escapes.
		if a := u.root(x.Val); a != nil {
			u.escaped[a] = true
		}
		return
	}
	u.escapeOperands(in)
}

// escapeOperands marks the variables that are operands of in.
func (u *ssaUnit) escapeOperands(in ssa.Instruction) {
	for _, op := range in.Operands(nil) {
		if op == nil || *op == nil {
			continue
		}
		if a := u.root(*op); a != nil {
			u.escaped[a] = true
		}
	}
}

// readOf is the set that a load of the variable root yields in f: the union
// of its stores, or, in a deferred literal, the set at the return.
func (f *ssaFn) readOf(root *ssa.Alloc) errSet {
	src := f.u.allocs
	if root.Parent() != f.fn && f.u.deferOnly[f.fn] {
		src = f.u.rets
	}
	out := errSet{}
	out.putAll(src[root])
	return out
}

// resultAlloc returns the variable that a returned value is loaded from when
// the value is a load of a variable declared by f (a named or result
// variable of a function with defers or captures), else nil.
func (f *ssaFn) resultAlloc(v ssa.Value) *ssa.Alloc {
	un, ok := v.(*ssa.UnOp)
	if !ok || un.Op != token.MUL {
		return nil
	}
	a := f.u.root(un.X)
	if a == nil || a.Parent() != f.fn {
		return nil
	}
	return a
}

// valueAtReturn is the set that the variable a holds when the return in b
// executes: the value of the last store to it in b (the return's own store),
// narrowed as the block's environment says. Without such a store (a naked
// return), it is the union of the stores.
func (f *ssaFn) valueAtReturn(b *ssa.BasicBlock, a *ssa.Alloc) errSet {
	for i := len(b.Instrs) - 2; i >= 0; i-- {
		if st, ok := b.Instrs[i].(*ssa.Store); ok && st.Addr == a {
			return f.effective(st.Val, f.env[b])
		}
	}
	out := errSet{}
	out.putAll(f.u.allocs[a])
	return out
}

// updateRets recomputes rets for the result variables of the functions of
// the unit (see the package comment) and reports whether one changed.
func (u *ssaUnit) updateRets() bool {
	changed := false
	for _, f := range u.fns {
		byAlloc := map[*ssa.Alloc][]*ssa.BasicBlock{}
		for _, b := range f.rpo {
			if !f.done[b] || len(b.Instrs) == 0 {
				continue
			}
			r, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
			if !ok || f.errIdx < 0 || len(r.Results) <= f.errIdx {
				continue
			}
			if a := f.resultAlloc(r.Results[f.errIdx]); a != nil {
				byAlloc[a] = append(byAlloc[a], b)
			}
		}
		for a, blocks := range byAlloc {
			s := errSet{}
			for _, b := range blocks {
				s.putAll(f.valueAtReturn(b, a))
			}
			s.putAll(u.closureWrites[a])
			if u.escaped[a] {
				s.put(opaqueVariant())
			}
			if u.recovers {
				s.putAll(u.allocs[a])
			}
			cur := u.rets[a]
			if cur == nil {
				cur = errSet{}
				u.rets[a] = cur
			}
			if cur.putAll(s) {
				changed = true
			}
		}
	}
	return changed
}

// maxReachHops bounds the walk of reachingStore back through the blocks.
const maxReachHops = 64

// loadSet is the set that the load x of the variable root, in block b, reads in
// f. In the declaring function, a load reads the store that reaches it: the
// store on the single chain of predecessors that leads to it (reachingStore),
// joined with the closures' writes, which may run in between. Otherwise (the
// variable escapes, a recovered panic may return it, the chain has a join
// without a store, or f is a closure) it reads the union of the stores.
func (f *ssaFn) loadSet(x *ssa.UnOp, root *ssa.Alloc, b *ssa.BasicBlock) errSet {
	if root.Parent() == f.fn && !f.u.escaped[root] && !f.u.recovers {
		if v, blk, ok := reachingStore(x, root, b); ok {
			out := errSet{}
			out.putAll(f.effective(v, f.env[blk]))
			out.putAll(f.u.closureWrites[root])
			return out
		}
	}
	return f.readOf(root)
}

// reachingStore finds the last store to a before the load x in block b, on the
// unique path of single-predecessor blocks that leads to x. It returns the
// stored value and the block of the store. ok is false when the path reaches a
// block with several predecessors before a store.
func reachingStore(x ssa.Instruction, a *ssa.Alloc, b *ssa.BasicBlock) (ssa.Value, *ssa.BasicBlock, bool) {
	end := len(b.Instrs)
	for i, in := range b.Instrs {
		if in == x {
			end = i
			break
		}
	}
	cur, before := b, end
	for hop := 0; hop < maxReachHops; hop++ {
		for i := before - 1; i >= 0; i-- {
			if st, ok := cur.Instrs[i].(*ssa.Store); ok && st.Addr == a {
				return st.Val, cur, true
			}
		}
		if len(cur.Preds) != 1 {
			return nil, nil, false
		}
		cur = cur.Preds[0]
		before = len(cur.Instrs)
	}
	return nil, nil, false
}
