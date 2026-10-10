package errset

// Interface contracts: a //effectgo:errors directive on an interface method,
// or on the interface type (applying to all its methods, a method-level
// directive overriding it), declares the errors that the method's
// implementations may return. A call to a contracted method yields the
// declared set instead of "?", and every implementation is checked against
// the contract.

import (
	"go/ast"
	"go/token"
	"go/types"
)

// ifaceMethod is a contracted method of a named interface.
type ifaceMethod struct {
	recv    *types.Named // the interface that declares the method, or embeds it
	iface   *types.Interface
	method  *types.Func
	allowed []Variant
}

// collectInterfaceContracts reads the contracts of the interfaces declared in
// this package, records them in p.contracts and exports them as facts.
func (p *pkgState) collectInterfaceContracts() {
	for _, f := range p.pass.Files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok || it.Methods == nil {
					continue
				}
				typeCG := ts.Doc
				if typeCG == nil && len(gd.Specs) == 1 {
					typeCG = gd.Doc
				}
				typeDirs := p.contractDirectives(typeCG)
				hasTypeContract := len(typeDirs) > 0
				var typeAllowed []Variant
				if hasTypeContract {
					typeAllowed, _ = p.resolveContract(typeDirs, ts.Name.Pos(), ts.Name.Name)
				}
				for _, field := range it.Methods.List {
					if len(field.Names) != 1 {
						continue // embedded interface
					}
					fn, ok := p.pass.TypesInfo.Defs[field.Names[0]].(*types.Func)
					if !ok {
						continue
					}
					own := p.contractDirectives(field.Doc)
					var allowed []Variant
					switch {
					case len(own) > 0:
						allowed, _ = p.resolveContract(own, field.Names[0].Pos(), fn.Name())
					case hasTypeContract:
						allowed = typeAllowed
					default:
						continue
					}
					p.contracts[fn] = allowed
					p.pass.ExportObjectFact(fn, &ContractFact{Allowed: allowed})
				}
			}
		}
	}
}

// contractDirectives returns the //effectgo:errors directives of cg. Other
// directive kinds are not valid on interfaces and are reported.
func (p *pkgState) contractDirectives(cg *ast.CommentGroup) []directive {
	var out []directive
	for _, d := range directivesOf(cg) {
		if d.kind == "errors" {
			out = append(out, d)
			continue
		}
		if effectsKinds[d.kind] || otherKinds[d.kind] {
			continue // a contract of the effects analyzer, or a keyword of mustuse or exhaustive
		}
		p.pass.Reportf(d.pos, "unknown directive //effectgo:%s on an interface method or type", d.kind)
	}
	return out
}

// contractOf returns the contract of an interface method declared in this
// package or in an analyzed dependency.
func (p *pkgState) contractOf(m *types.Func) ([]Variant, bool) {
	m = m.Origin()
	if a, ok := p.contracts[m]; ok {
		return a, true
	}
	if m.Pkg() == nil || m.Pkg() == p.pass.Pkg {
		return nil, false
	}
	var f ContractFact
	if p.pass.ImportObjectFact(m, &f) {
		return f.Allowed, true
	}
	return nil, false
}

// contractedMethods lists every contracted method of a named interface that is
// declared in this package or in any transitively imported package.
func (p *pkgState) contractedMethods() []ifaceMethod {
	var out []ifaceMethod
	seen := map[*types.Package]bool{}
	var walk func(pkg *types.Package)
	walk = func(pkg *types.Package) {
		if seen[pkg] {
			return
		}
		seen[pkg] = true
		scope := pkg.Scope()
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}
			it, ok := named.Underlying().(*types.Interface)
			if !ok {
				continue
			}
			for i := 0; i < it.NumMethods(); i++ {
				m := it.Method(i)
				if allowed, ok := p.contractOf(m); ok {
					out = append(out, ifaceMethod{recv: named, iface: it, method: m, allowed: allowed})
				}
			}
		}
		for _, imp := range pkg.Imports() {
			walk(imp)
		}
	}
	walk(p.pass.Pkg)
	return out
}

// inferredOf is the inferred error set of a method, from this package or from a fact.
func (p *pkgState) inferredOf(f *types.Func) (errSet, bool) {
	if f.Pkg() == p.pass.Pkg {
		w, ok := p.work[f]
		if !ok {
			return nil, false
		}
		return w.errs, true
	}
	var fact ErrFact
	if !p.pass.ImportObjectFact(f, &fact) {
		return nil, false
	}
	return setOf(fact.Errors...), true
}

// checkConformance reports, for every named non-interface type T declared in
// this package and every contracted method m of an interface I that T (or *T)
// implements, each variant that the implementation of m may return and that
// the contract of m does not allow.
func (p *pkgState) checkConformance() {
	methods := p.contractedMethods()
	if len(methods) == 0 {
		return
	}
	scope := p.pass.Pkg.Scope()
	reported := map[string]bool{}
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		T, ok := tn.Type().(*types.Named)
		if !ok || types.IsInterface(T) {
			continue
		}
		ptr := types.NewPointer(T)
		mset := types.NewMethodSet(ptr)
		for _, cm := range methods {
			if !types.Implements(ptr, cm.iface) {
				continue
			}
			sel := mset.Lookup(cm.method.Pkg(), cm.method.Name())
			if sel == nil {
				continue
			}
			impl, ok := sel.Obj().(*types.Func)
			if !ok {
				continue
			}
			impl = impl.Origin()
			set, ok := p.inferredOf(impl)
			if !ok {
				continue
			}
			star := ""
			if recv := impl.Type().(*types.Signature).Recv(); recv != nil {
				if _, isPtr := types.Unalias(recv.Type()).(*types.Pointer); isPtr {
					star = "*"
				}
			}
			for _, v := range displayOrder(set.list()) {
				if allowedBy(v, cm.allowed) {
					continue
				}
				key := T.Obj().Name() + "." + impl.Name() + "|" + cm.recv.Obj().Name() + "." + cm.method.Name() + "|" + v.Key
				if reported[key] {
					continue
				}
				reported[key] = true
				pos := tn.Pos()
				if impl.Pkg() == p.pass.Pkg {
					pos = impl.Pos()
				}
				p.pass.Reportf(pos, "(%s%s).%s violates contract of (%s).%s: may return %s%s",
					star, T.Obj().Name(), impl.Name(), cm.recv.Obj().Name(), cm.method.Name(), v.Name, via(v.Chain))
			}
		}
	}
}
