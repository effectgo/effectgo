package errset

// Guard narrowing (M2). A branch on a test of an error value narrows that
// value on each of its edges:
//
//	errors.Is(v, S)   true: v is S or a wrapper (a type with Unwrap, Is or As
//	                  methods, or an unknown origin); false: S is removed.
//	v == S            true: v is S (or unknown); false: S is removed. No
//	                  wrappers: == does not unwrap.
//	v == nil          the nil edge carries the empty set.
//	errors.As(v, &x)  with a concrete T: true keeps T (and wrappers, unknown
//	                  origins); false removes T. With an interface I: true keeps
//	                  the variants whose method set implements I (wrappers and
//	                  unknown origins too); false removes the known variants
//	                  that implement I.
//	v.(T) (comma ok)  as errors.As, without the chain: no wrappers.
//	P(v)              a predicate with a TestFact (see testFact): the test it
//	                  names, on its argument.
//	!c                the same test with the edges swapped.
//
// A switch compiles to an If chain, so its cases narrow the same way.
// Narrowing is a per-edge transfer on the environment of a block (see
// edgeEnv in ssaflow.go); a Phi takes the narrowed set of each incoming edge.

import (
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// TestFact is exported on a predicate function func(error, …) bool whose
// result is a test of its first parameter: errors.Is with a sentinel
// (Is true, Key the sentinel), or errors.As with a concrete type (Key the
// type) or an interface (Iface, Methods). See predicateFact.
type TestFact struct {
	Is      bool
	Key     string
	Iface   bool
	Methods []string
}

// AFact marks TestFact as an analysis fact.
func (*TestFact) AFact() {}

func (f *TestFact) String() string {
	switch {
	case f.Is:
		return "errset-test: errors.Is " + f.Key
	case f.Iface:
		return "errset-test: errors.As iface " + strings.Join(f.Methods, ";")
	}
	return "errset-test: errors.As " + f.Key
}

// target is the type that an errors.As or a type assertion tests: a concrete
// error type (key) or an interface (its method set).
type target struct {
	key     string
	iface   bool
	methods []string
}

// errorMethods is the method set of a fresh error (errors.New's type).
var errorMethods = []string{"Error()(string)"}

// narrowEdge narrows the environment *e of the edge p -> b by the branch that
// ends p. The map may be shared with another edge, so it is copied before the
// first write. It reports whether the edge narrows at all.
func (f *ssaFn) narrowEdge(p, b *ssa.BasicBlock, e *map[ssa.Value]errSet) bool {
	if len(p.Instrs) == 0 || len(p.Succs) != 2 {
		return false
	}
	ifi, ok := p.Instrs[len(p.Instrs)-1].(*ssa.If)
	if !ok {
		return false
	}
	tb, fb := p.Succs[0], p.Succs[1]
	if tb == fb || (b != tb && b != fb) {
		return false
	}
	cp := make(map[ssa.Value]errSet, len(*e)+1)
	for v, s := range *e {
		cp[v] = s
	}
	*e = cp
	f.narrowCond(ifi.Cond, b == tb, cp)
	return true
}

// narrowCond narrows e for the condition cond being positive (true) or not.
func (f *ssaFn) narrowCond(cond ssa.Value, positive bool, e map[ssa.Value]errSet) {
	switch c := cond.(type) {
	case *ssa.UnOp:
		if c.Op == token.NOT {
			f.narrowCond(c.X, !positive, e)
		}
	case *ssa.Extract:
		ta, ok := c.Tuple.(*ssa.TypeAssert)
		if !ok || !ta.CommaOk || c.Index != 1 {
			return
		}
		if tgt, ok := typeTarget(ta.AssertedType); ok {
			f.narrowAt(ta.X, e, func(s errSet) errSet { return narrowAs(s, tgt, positive, false) })
		}
	case *ssa.BinOp:
		if c.Op != token.EQL && c.Op != token.NEQ {
			return
		}
		eq := (c.Op == token.EQL) == positive
		x, y := c.X, c.Y
		if isNilConst(x) {
			x, y = y, x
		}
		if isNilConst(y) {
			if eq {
				f.narrowAt(x, e, func(errSet) errSet { return errSet{} })
			}
			return
		}
		subject, key, ok := sentinelComparison(x, y)
		if !ok {
			return
		}
		f.narrowAt(subject, e, func(s errSet) errSet { return narrowEq(s, key, eq) })
	case *ssa.Call:
		f.narrowCall(c, positive, e)
	}
}

// sentinelComparison reports which operand is a sentinel and returns the other
// one as the subject of the comparison.
func sentinelComparison(x, y ssa.Value) (subject ssa.Value, key string, ok bool) {
	if k, ok := sentinelOfValue(y); ok {
		return x, k, true
	}
	if k, ok := sentinelOfValue(x); ok {
		return y, k, true
	}
	return nil, "", false
}

// narrowCall narrows by a call that tests its first argument: errors.Is,
// errors.As, or a predicate with a TestFact.
func (f *ssaFn) narrowCall(c *ssa.Call, positive bool, e map[ssa.Value]errSet) {
	fn := c.Call.StaticCallee()
	if fn == nil {
		return
	}
	obj, ok := fn.Object().(*types.Func)
	if !ok {
		return
	}
	obj = obj.Origin()
	args := c.Call.Args
	if obj.Pkg() != nil && obj.Pkg().Path() == "errors" && len(args) == 2 {
		switch obj.Name() {
		case "Is":
			if key, ok := sentinelOfValue(args[1]); ok {
				f.narrowAt(args[0], e, func(s errSet) errSet { return narrowIs(s, key, positive) })
			}
		case "As":
			if tgt, ok := asTarget(args[1]); ok {
				f.narrowAt(args[0], e, func(s errSet) errSet { return narrowAs(s, tgt, positive, true) })
			}
		}
		return
	}
	if len(args) < 1 {
		return
	}
	if t, ok := f.u.p.testOf(obj); ok {
		f.narrowAt(args[0], e, func(s errSet) errSet {
			if t.Is {
				return narrowIs(s, t.Key, positive)
			}
			return narrowAs(s, target{key: t.Key, iface: t.Iface, methods: t.Methods}, positive, true)
		})
	}
}

// narrowAt replaces the set of x in the environment e by narrow applied to the
// set x has at that point: its entry in e, else its definition.
func (f *ssaFn) narrowAt(x ssa.Value, e map[ssa.Value]errSet, narrow func(errSet) errSet) {
	cur, ok := e[x]
	if !ok {
		cur = f.defSet(x)
	}
	e[x] = intersectSet(narrow(cur), f.defSet(x))
}

// narrowIs is the set of errors.Is(v, S) on the given edge.
func narrowIs(s errSet, key string, positive bool) errSet {
	out := errSet{}
	for k, v := range s {
		if positive {
			if v.Key == key || v.Kind == KindOpaque || v.Kind == KindParam || v.Kind == KindIface || isWrapper(v) {
				out[k] = v
			}
		} else if v.Key != key {
			out[k] = v
		}
	}
	return out
}

// narrowEq is the set of v == S on the given edge.
func narrowEq(s errSet, key string, positive bool) errSet {
	out := errSet{}
	for k, v := range s {
		if positive {
			if v.Key == key || v.Kind == KindOpaque || v.Kind == KindParam || v.Kind == KindIface {
				out[k] = v
			}
		} else if v.Key != key {
			out[k] = v
		}
	}
	return out
}

// narrowAs is the set of errors.As(v, &x) (chain true) or of a type assertion
// (chain false) on the given edge.
func narrowAs(s errSet, t target, positive, chain bool) errSet {
	out := errSet{}
	for k, v := range s {
		if positive {
			if t.matches(v) || v.Kind == KindOpaque || v.Kind == KindParam || v.Kind == KindIface || (chain && isWrapper(v)) {
				out[k] = v
			}
		} else if !t.matches(v) {
			out[k] = v
		}
	}
	return out
}

// matches reports whether the variant v is known to have the type of t.
func (t target) matches(v Variant) bool {
	if t.iface {
		switch v.Kind {
		case KindType, KindSentinel:
			return implementsAll(v.Methods, t.methods)
		case KindFresh:
			return implementsAll(errorMethods, t.methods)
		}
		return false
	}
	return v.Key == t.key
}

// isWrapper reports whether a variant may wrap another error: a concrete
// type or sentinel with an Unwrap, Is or As method.
func isWrapper(v Variant) bool {
	if v.Kind != KindType && v.Kind != KindSentinel {
		return false
	}
	for _, m := range v.Methods {
		if strings.HasPrefix(m, "Unwrap(") || strings.HasPrefix(m, "Is(") || strings.HasPrefix(m, "As(") {
			return true
		}
	}
	return false
}

// typeTarget is the target of a type assertion to t: a concrete error type or
// an interface.
func typeTarget(t types.Type) (target, bool) {
	if types.IsInterface(t) {
		return target{iface: true, methods: methodsOf(t)}, true
	}
	if n := namedErrorType(t); n != nil {
		return target{key: objKey(n.Obj())}, true
	}
	return target{}, false
}

// asTarget is the target of an errors.As call's second argument: a pointer to
// a variable of type T, boxed in an any (MakeInterface of the variable's
// address, whose type is **T).
func asTarget(arg ssa.Value) (target, bool) {
	mi, ok := arg.(*ssa.MakeInterface)
	if !ok {
		return target{}, false
	}
	ptr, ok := mi.X.Type().Underlying().(*types.Pointer)
	if !ok {
		return target{}, false
	}
	return typeTarget(ptr.Elem())
}

// sentinelOfValue returns the key of the sentinel that v loads, looking
// through a conversion to an interface.
func sentinelOfValue(v ssa.Value) (string, bool) {
	for {
		switch x := v.(type) {
		case *ssa.MakeInterface:
			v = x.X
			continue
		case *ssa.ChangeInterface:
			v = x.X
			continue
		case *ssa.UnOp:
			if x.Op != token.MUL {
				return "", false
			}
			g, ok := x.X.(*ssa.Global)
			if !ok {
				return "", false
			}
			sv, ok := sentinelKey(g.Object())
			if !ok {
				return "", false
			}
			return objKey(sv), true
		}
		return "", false
	}
}

// isNilConst reports whether v is the nil constant.
func isNilConst(v ssa.Value) bool {
	c, ok := v.(*ssa.Const)
	return ok && c.IsNil()
}

// testOf returns the TestFact of a function: a predicate of this package
// (computed by predicateFact) or of an imported package (its exported fact).
func (p *pkgState) testOf(fn *types.Func) (TestFact, bool) {
	if fn.Pkg() == p.pass.Pkg {
		t, ok := p.tests[fn]
		return t, ok
	}
	var t TestFact
	if p.pass.ImportObjectFact(fn, &t) {
		return t, true
	}
	return TestFact{}, false
}

// predicateFact reads the test that a func(error) bool performs on its
// parameter. Every return must be a constant false, a test of the parameter,
// or the constant true reached only through the positive edge of such a test.
// All tests must be the same; otherwise the function has no fact.
func (p *pkgState) predicateFact(fn *ssa.Function) (TestFact, bool) {
	sig := fn.Signature
	if sig.Recv() != nil || sig.Params().Len() != 1 || sig.Results().Len() != 1 || len(fn.Params) != 1 {
		return TestFact{}, false
	}
	if !implementsError(sig.Params().At(0).Type()) {
		return TestFact{}, false
	}
	if b, ok := sig.Results().At(0).Type().Underlying().(*types.Basic); !ok || b.Kind() != types.Bool {
		return TestFact{}, false
	}
	param := fn.Params[0]
	var fact TestFact
	have := false
	for _, b := range fn.Blocks {
		if len(b.Instrs) == 0 {
			continue
		}
		r, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return)
		if !ok || len(r.Results) != 1 {
			continue
		}
		var t TestFact
		switch x := r.Results[0].(type) {
		case *ssa.Call:
			tf, ok := p.testOfCall(x, param)
			if !ok {
				return TestFact{}, false
			}
			t = tf
		case *ssa.Const:
			if x.Value == nil || x.Value.Kind() != constant.Bool {
				return TestFact{}, false
			}
			if !constant.BoolVal(x.Value) {
				continue
			}
			if len(b.Preds) != 1 {
				return TestFact{}, false
			}
			pb := b.Preds[0]
			ifi, ok := pb.Instrs[len(pb.Instrs)-1].(*ssa.If)
			if !ok || len(pb.Succs) != 2 || pb.Succs[0] != b || pb.Succs[1] == b {
				return TestFact{}, false
			}
			call, ok := ifi.Cond.(*ssa.Call)
			if !ok {
				return TestFact{}, false
			}
			tf, ok := p.testOfCall(call, param)
			if !ok {
				return TestFact{}, false
			}
			t = tf
		default:
			return TestFact{}, false
		}
		if have && !reflect.DeepEqual(fact, t) {
			return TestFact{}, false
		}
		fact, have = t, true
	}
	return fact, have
}

// testOfCall is the test that a call performs on the value param: errors.Is or
// errors.As on it, or another predicate with a fact.
func (p *pkgState) testOfCall(c *ssa.Call, param ssa.Value) (TestFact, bool) {
	fn := c.Call.StaticCallee()
	if fn == nil || len(c.Call.Args) == 0 || c.Call.Args[0] != param {
		return TestFact{}, false
	}
	obj, ok := fn.Object().(*types.Func)
	if !ok {
		return TestFact{}, false
	}
	obj = obj.Origin()
	if obj.Pkg() != nil && obj.Pkg().Path() == "errors" && len(c.Call.Args) == 2 {
		switch obj.Name() {
		case "Is":
			if key, ok := sentinelOfValue(c.Call.Args[1]); ok {
				return TestFact{Is: true, Key: key}, true
			}
		case "As":
			if tgt, ok := asTarget(c.Call.Args[1]); ok {
				return TestFact{Key: tgt.key, Iface: tgt.iface, Methods: tgt.methods}, true
			}
		}
		return TestFact{}, false
	}
	return p.testOf(obj)
}

// exportTests computes the TestFact of each predicate of the package and
// exports it. Only the SSA engine narrows, so only it has the facts.
func (p *pkgState) exportTests(units []*fnUnit) {
	p.tests = map[*types.Func]TestFact{}
	for _, u := range units {
		fn := p.ssaFuncOf(u)
		if fn == nil {
			continue
		}
		if t, ok := p.predicateFact(fn); ok {
			p.tests[u.obj] = t
			p.pass.ExportObjectFact(u.obj, &t)
		}
	}
}
