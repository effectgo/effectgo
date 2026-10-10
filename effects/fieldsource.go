package effects

// Field sources. A stub "effects field:(pkgpath.T).F <effects>" says that the
// value of field F of a named struct type T, read through a value of type T,
// performs the fixed effects of the rule. The stdlib reader and writer fields
// of net/http are the case: (*net/http.Request).Body carries the request's
// network data, and (*net/http.Response).Body the response stream (the doc
// comments are at net/http/request.go:174-188 and net/http/response.go:55-72).
//
// The rule applies in three places, and nowhere else:
//
//   - a method called on the field value: r.Body.Read(p);
//   - an argument of a via item, or the value that a stores item keeps, when
//     the argument is the field load: json.NewDecoder(r.Body);
//   - the same, through a local variable with exactly one assignment, from the
//     field load, and that is not address-taken: body := r.Body. Such a local
//     must have an interface or pointer type, so that no method can change it
//     through its address.
//
// Any other value of the same type, or a field of another type, is evaluated as
// before (usually unknown). A value that the analysis cannot see the origin of
// is not a field source, so a rule over-approximates when the field holds an
// in-memory value (a test that substitutes one): the effect is charged, not
// lost.

import (
	"go/ast"
	"go/types"
)

// fieldRef is a field load that a field source rule names: the field name of
// the struct type owner, with the rule's fixed effects.
type fieldRef struct {
	owner *types.Named
	field string
	effs  []string
}

// fieldLoad returns the field source rule of the expression e when it is a
// field load x.F (possibly parenthesized) and the struct type that declares F
// has a field source rule. The owner is the struct that declares F, which for
// a promoted field is the embedded type, so a field promoted through an
// embedded net/http.Request matches the rule of Request.
func (c *checker) fieldLoad(e ast.Expr) (fieldRef, bool) {
	sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
	if !ok {
		return fieldRef{}, false
	}
	s := c.pass.TypesInfo.Selections[sel]
	if s == nil || s.Kind() != types.FieldVal {
		return fieldRef{}, false
	}
	owner, ok := fieldOwner(s)
	if !ok {
		return fieldRef{}, false
	}
	effs, ok := c.stubs.lookup(fieldKey(owner, sel.Sel.Name))
	if !ok {
		return fieldRef{}, false
	}
	return fieldRef{owner: owner, field: sel.Sel.Name, effs: effs}, true
}

// fieldOwner returns the named struct type that declares the field of the
// selection s: the type reached by following the embedding path to the last
// hop. It is false for an unnamed struct, or for a path through a non-struct.
func fieldOwner(s *types.Selection) (*types.Named, bool) {
	t := s.Recv()
	idx := s.Index()
	for _, i := range idx[:len(idx)-1] {
		st, ok := structOf(t)
		if !ok || i >= st.NumFields() {
			return nil, false
		}
		t = st.Field(i).Type()
	}
	return namedOf(t)
}

// structOf returns the struct of t, through one pointer and a named type.
func structOf(t types.Type) (*types.Struct, bool) {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	st, ok := types.Unalias(t).Underlying().(*types.Struct)
	return st, ok
}

// namedOf returns the named type of t, through one pointer, or false.
func namedOf(t types.Type) (*types.Named, bool) {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return nil, false
	}
	return n.Origin(), true
}

// fieldKey is the stub key of field name of the named struct type owner. It is
// the form of stubs.Target.String for a field source: "field:(pkg.T).F".
func fieldKey(owner *types.Named, name string) string {
	o := owner.Obj()
	if o.Pkg() == nil {
		return ""
	}
	return "field:(" + o.Pkg().Path() + "." + o.Name() + ")." + name
}

// fieldStep names a field source in a witness: "(Request).Body [field]", or
// "(Request).Body.Read [field]" for a method called on the field value.
func fieldStep(ref fieldRef, method string) Step {
	name := "(" + ref.owner.Obj().Name() + ")." + ref.field
	if method != "" {
		name += "." + method
	}
	s := Step{Name: name + " [field]"}
	if p := ref.owner.Obj().Pkg(); p != nil {
		s.Pkg, s.Qual = p.Path(), p.Name()
	}
	return s
}

// fieldArg returns the field source rule of an argument of a call: the field
// load itself, or a trusted local variable whose one assignment is a field load.
func (c *checker) fieldArg(arg ast.Expr) (fieldRef, bool) {
	if ref, ok := c.fieldLoad(arg); ok {
		return ref, true
	}
	id, ok := ast.Unparen(arg).(*ast.Ident)
	if !ok {
		return fieldRef{}, false
	}
	v, ok := c.pass.TypesInfo.Uses[id].(*types.Var)
	if !ok || !isLocalVar(v) {
		return fieldRef{}, false
	}
	lf := c.locals()
	if lf.addr[v] || len(lf.defs[v]) != 1 || lf.defs[v][0] == nil || !interfaceOrPointer(v.Type()) {
		return fieldRef{}, false
	}
	return c.fieldLoad(lf.defs[v][0])
}

// interfaceOrPointer reports whether t is an interface or a pointer type.
func interfaceOrPointer(t types.Type) bool {
	switch types.Unalias(t).Underlying().(type) {
	case *types.Interface, *types.Pointer:
		return true
	}
	return false
}

// fieldCall returns the field source rule of a method call whose receiver is a
// field load (r.Body.Read(p)). A method expression call is not one.
func (c *checker) fieldCall(call *ast.CallExpr) (fieldRef, string, bool) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return fieldRef{}, "", false
	}
	if s := c.pass.TypesInfo.Selections[sel]; s == nil || s.Kind() != types.MethodVal {
		return fieldRef{}, "", false
	}
	ref, ok := c.fieldLoad(sel.X)
	if !ok {
		return fieldRef{}, "", false
	}
	return ref, sel.Sel.Name, true
}

// fieldHits returns the effects of a field source rule as the via hits of an
// argument of a call whose callee is self.
func fieldHits(self Step, ref fieldRef) []viaHit {
	out := make([]viaHit, 0, len(ref.effs))
	for _, e := range ref.effs {
		out = append(out, viaHit{e, Witness{self, fieldStep(ref, "")}})
	}
	return out
}

// methodOfField is used by collectCall: a method call on a field source value
// has the rule's effects, instead of the interface contract or unknown.
func (c *checker) methodOfField(st *fnState, call *ast.CallExpr) bool {
	ref, method, ok := c.fieldCall(call)
	if !ok {
		return false
	}
	step := fieldStep(ref, method)
	for _, e := range ref.effs {
		st.offer(e, prepend(st.self, Witness{step}))
	}
	return true
}
