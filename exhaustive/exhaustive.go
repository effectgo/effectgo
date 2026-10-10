// Package exhaustive is the effectgo analyzer for Rust-style exhaustive
// matching on sealed sum types.
//
// Two kinds of type are sealed:
//
//   - An interface is sealed when it has an unexported method declared by its
//     own package (the Go marker-method pattern: only that package can implement
//     it), or when it carries //effectgo:sealed. Its variants are the named
//     types of the package whose T or *T implements it. An interface that embeds
//     a sealed interface is checked against that interface's variants.
//   - A named integer or string type carrying //effectgo:sealed is an enum. Its
//     variants are the distinct values of the package-level constants of that
//     type.
//
// A type switch on a sealed interface, or an expression switch on an enum, with
// no default clause must have a case for every variant. Variants are computed
// from the non-test files of the declaring package, so a package and its
// importers agree on them (see docs/journal/11-p4a-mustuse-exhaustive.md).
package exhaustive

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/faintghost/effectgo/internal/dirparse"
)

// Directive marks an interface or an enum type as sealed.
const Directive = "//effectgo:sealed"

// keyword is the directive keyword of Directive.
const keyword = "sealed"

// sealedFact is exported on the *types.TypeName of a sealed interface. Pkg is
// the path of the declaring package, Variants the names of its variant types.
// gob needs exported fields.
type sealedFact struct {
	Pkg      string
	Variants []string
}

func (*sealedFact) AFact() {}
func (f *sealedFact) String() string {
	return "sealed(" + strings.Join(f.Variants, ",") + ")"
}

// enumFact is exported on the *types.TypeName of a sealed enum. Consts lists
// the package-level constants of the type, sorted by name.
type enumFact struct {
	Pkg    string
	Consts []enumConst
}

// enumConst is one constant of an enum. Value is constant.Value.ExactString().
type enumConst struct {
	Name     string
	Value    string
	Exported bool
}

func (*enumFact) AFact() {}
func (f *enumFact) String() string {
	var names []string
	for _, c := range f.Consts {
		names = append(names, c.Name)
	}
	return "enum(" + strings.Join(names, ",") + ")"
}

// Analyzer is the exhaustive analyzer.
var Analyzer = &analysis.Analyzer{
	Name:      "exhaustive",
	Doc:       "report non-exhaustive type switches on sealed interfaces and expression switches on sealed enums",
	FactTypes: []analysis.Fact{new(sealedFact), new(enumFact)},
	Run:       run,
}

func run(pass *analysis.Pass) (any, error) {
	exportSealed(pass)

	// The switches are visited as ast.Inspect visits the files. The inspector is
	// not required: the checker keeps the result of every action of a dependency
	// alive, and one inspector per package was a large part of the standalone
	// run's memory (see docs/journal/17-p7b-body-fields-memory.md).
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.TypeSwitchStmt:
				checkTypeSwitch(pass, n)
			case *ast.SwitchStmt:
				checkEnumSwitch(pass, n)
			}
			return true
		})
	}
	return nil, nil
}

// isTestFile reports whether pos lies in a _test.go file. Test-only declarations
// are not variants (see exportSealed).
func isTestFile(pass *analysis.Pass, pos token.Pos) bool {
	return strings.HasSuffix(pass.Fset.Position(pos).Filename, "_test.go")
}

// exportSealed exports a fact for every sealed interface and enum declared in a
// non-test file of this package. A type declared in a test file is not sealed
// for the importers, and its switches are not checked.
func exportSealed(pass *analysis.Pass) {
	for _, file := range pass.Files {
		if isTestFile(pass, file.Pos()) {
			continue
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				marks := markComments(ts.Doc)
				if len(gd.Specs) == 1 {
					marks = append(marks, markComments(gd.Doc)...)
				}
				obj, _ := pass.TypesInfo.Defs[ts.Name].(*types.TypeName)
				if obj == nil {
					continue
				}
				for _, c := range marks {
					reportArgs(pass, c)
				}
				directive := len(marks) > 0
				if obj.IsAlias() {
					if directive {
						pass.Reportf(ts.Name.Pos(), "%s: %s applies to a defined type, not an alias; put it on the type the alias names", ts.Name.Name, Directive)
					}
					continue
				}
				named, ok := obj.Type().(*types.Named)
				if !ok {
					continue
				}
				if iface, ok := named.Underlying().(*types.Interface); ok {
					exportInterface(pass, obj, iface, directive)
					continue
				}
				if directive {
					exportEnum(pass, obj, named)
				}
			}
		}
	}
}

// exportInterface exports a sealedFact for an interface that is sealed by its
// directive or by its own unexported marker method.
func exportInterface(pass *analysis.Pass, obj *types.TypeName, iface *types.Interface, directive bool) {
	marker := hasOwnUnexportedMethod(pass.Pkg, iface)
	if !directive && !marker {
		return
	}
	if !marker {
		pass.Reportf(obj.Pos(), "sealed interface %s must have an unexported method", obj.Name())
		return
	}
	vs := variants(pass, pass.Pkg, iface)
	names := make([]string, len(vs))
	for i, v := range vs {
		names[i] = v.name
	}
	pass.ExportObjectFact(obj, &sealedFact{Pkg: pass.Pkg.Path(), Variants: names})
}

// exportEnum exports an enumFact for a named integer or string type.
func exportEnum(pass *analysis.Pass, obj *types.TypeName, named *types.Named) {
	basic, ok := named.Underlying().(*types.Basic)
	if !ok || basic.Info()&(types.IsInteger|types.IsString) == 0 {
		pass.Reportf(obj.Pos(), "%s: %s applies to interfaces and to integer or string types", obj.Name(), Directive)
		return
	}
	fact := &enumFact{Pkg: pass.Pkg.Path()}
	for _, name := range pass.Pkg.Scope().Names() {
		c, ok := pass.Pkg.Scope().Lookup(name).(*types.Const)
		if !ok || isTestFile(pass, c.Pos()) || !types.Identical(c.Type(), named) {
			continue
		}
		fact.Consts = append(fact.Consts, enumConst{Name: name, Value: c.Val().ExactString(), Exported: c.Exported()})
	}
	pass.ExportObjectFact(obj, fact)
}

// markComments returns the comments of cg that carry the sealed keyword. The keyword
// is read as dirparse reads it, so a trailing // comment after it is not an argument.
func markComments(cg *ast.CommentGroup) []*ast.Comment {
	if cg == nil {
		return nil
	}
	var out []*ast.Comment
	for _, c := range cg.List {
		if kw, _, ok := dirparse.Parse(c.Text); ok && kw == keyword {
			out = append(out, c)
		}
	}
	return out
}

// reportArgs reports a sealed directive with arguments, such as "//effectgo:sealed
// extra". The directive still applies: the argument is an error, as it is for pure in
// effects.
func reportArgs(pass *analysis.Pass, c *ast.Comment) {
	if _, args, _ := dirparse.Parse(c.Text); len(args) > 0 {
		pass.Reportf(c.Pos(), "%s takes no arguments", Directive)
	}
}

// hasOwnUnexportedMethod reports whether iface has an unexported method declared
// by pkg. An unexported method inherited from another package does not count:
// that package, not this one, decides who can implement it.
func hasOwnUnexportedMethod(pkg *types.Package, iface *types.Interface) bool {
	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		if !m.Exported() && m.Pkg() == pkg {
			return true
		}
	}
	return false
}

// variantRef is one variant of a sealed interface: the named type T (the origin
// of a generic type), identified by package path and name.
type variantRef struct {
	pkg, name string
	typ       *types.Named
}

func (v variantRef) key() string { return v.pkg + "." + v.name }

// variants returns the variants of iface declared in pkg, sorted by name. Only
// types of non-test files count. A generic type is one variant whatever its
// instances. Aliases are not variants: the type they name is.
func variants(pass *analysis.Pass, pkg *types.Package, iface *types.Interface) []variantRef {
	var out []variantRef
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() || isTestFile(pass, tn.Pos()) {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok || types.IsInterface(named) {
			continue
		}
		if types.Implements(named, iface) || types.Implements(types.NewPointer(named), iface) {
			out = append(out, variantRef{pkg: pkg.Path(), name: name, typ: named})
		}
	}
	return out
}

// sealedOf returns the variants of a sealed interface type t, or ok=false when t
// is not sealed. A named interface that is not sealed itself is checked through
// the sealed interfaces it embeds: its variants are those of the embedded ones
// that implement it too.
func sealedOf(pass *analysis.Pass, t types.Type, depth int) (name string, vs []variantRef, ok bool) {
	t = types.Unalias(t)
	named, isNamed := t.(*types.Named)
	if isNamed {
		var fact sealedFact
		if pass.ImportObjectFact(named.Obj(), &fact) {
			return named.Obj().Name(), lookupVariants(named.Obj().Pkg(), fact.Variants), true
		}
	}
	iface, isIface := t.Underlying().(*types.Interface)
	if !isIface || depth > 8 {
		return "", nil, false
	}
	seen := map[string]bool{}
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		_, base, ok := sealedOf(pass, iface.EmbeddedType(i), depth+1)
		if !ok {
			continue
		}
		for _, v := range base {
			if seen[v.key()] {
				continue
			}
			seen[v.key()] = true
			if implementsIface(v.typ, iface) {
				vs = append(vs, v)
			}
		}
	}
	if len(vs) == 0 {
		return "", nil, false
	}
	sortVariants(vs)
	if isNamed {
		return named.Obj().Name(), vs, true
	}
	return "interface", vs, true
}

// lookupVariants resolves the variant names of a sealed fact to variantRefs in
// the scope of the declaring package pkg. A name that does not resolve to a
// named type is dropped.
func lookupVariants(pkg *types.Package, names []string) []variantRef {
	var out []variantRef
	if pkg == nil {
		return nil
	}
	for _, name := range names {
		tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		out = append(out, variantRef{pkg: pkg.Path(), name: name, typ: named})
	}
	return out
}

func implementsIface(named *types.Named, iface *types.Interface) bool {
	return types.Implements(named, iface) || types.Implements(types.NewPointer(named), iface)
}

func sortVariants(vs []variantRef) {
	sort.Slice(vs, func(i, j int) bool { return vs[i].key() < vs[j].key() })
}

// checkTypeSwitch reports a type switch on a sealed interface that has no default
// clause and misses a variant. A case type covers the variants it names, and an
// interface case covers every variant that implements it.
func checkTypeSwitch(pass *analysis.Pass, ts *ast.TypeSwitchStmt) {
	x := typeSwitchOperand(ts)
	if x == nil {
		return
	}
	name, vs, ok := sealedOf(pass, pass.TypesInfo.TypeOf(x), 0)
	if !ok {
		return
	}
	covered := map[string]bool{}
	for _, stmt := range ts.Body.List {
		cc := stmt.(*ast.CaseClause)
		if cc.List == nil { // default clause: every value is handled
			return
		}
		for _, e := range cc.List {
			t := types.Unalias(pass.TypesInfo.TypeOf(e))
			if p, ok := t.(*types.Pointer); ok {
				t = types.Unalias(p.Elem())
			}
			switch t := t.(type) {
			case *types.Named:
				if t.Obj().Pkg() != nil {
					covered[t.Origin().Obj().Pkg().Path()+"."+t.Origin().Obj().Name()] = true
				}
			case *types.Interface:
				for _, v := range vs {
					if implementsIface(v.typ, t) {
						covered[v.key()] = true
					}
				}
			}
		}
	}
	var missing []string
	for _, v := range vs {
		if !covered[v.key()] {
			missing = append(missing, v.name)
		}
	}
	if len(missing) > 0 {
		pass.Reportf(ts.Pos(), "missing cases in type switch on %s: %s", name, strings.Join(missing, ", "))
	}
}

// checkEnumSwitch reports an expression switch on a sealed enum that has no
// default clause and misses a value. A case that is not a constant makes the
// switch unknown, and it is not reported.
func checkEnumSwitch(pass *analysis.Pass, sw *ast.SwitchStmt) {
	if sw.Tag == nil {
		return
	}
	named, ok := types.Unalias(pass.TypesInfo.TypeOf(sw.Tag)).(*types.Named)
	if !ok {
		return
	}
	var fact enumFact
	if !pass.ImportObjectFact(named.Obj(), &fact) {
		return
	}
	samePkg := named.Obj().Pkg() == pass.Pkg
	covered := map[string]bool{}
	for _, stmt := range sw.Body.List {
		cc := stmt.(*ast.CaseClause)
		if cc.List == nil {
			return
		}
		for _, e := range cc.List {
			tv, ok := pass.TypesInfo.Types[e]
			if !ok || tv.Value == nil {
				return
			}
			covered[tv.Value.ExactString()] = true
		}
	}
	// A value is a variant when some constant of that value is visible here.
	// Each value is reported once, under the first visible name.
	reported := map[string]bool{}
	var missing []string
	for _, c := range fact.Consts {
		if !(c.Exported || samePkg) || covered[c.Value] || reported[c.Value] {
			continue
		}
		reported[c.Value] = true
		missing = append(missing, c.Name)
	}
	if len(missing) > 0 {
		pass.Reportf(sw.Pos(), "missing cases in switch on %s: %s", named.Obj().Name(), strings.Join(missing, ", "))
	}
}

// typeSwitchOperand returns X for both `switch X.(type)` and `switch v := X.(type)`.
func typeSwitchOperand(ts *ast.TypeSwitchStmt) ast.Expr {
	var e ast.Expr
	switch a := ts.Assign.(type) {
	case *ast.ExprStmt:
		e = a.X
	case *ast.AssignStmt:
		if len(a.Rhs) == 1 {
			e = a.Rhs[0]
		}
	}
	ta, ok := ast.Unparen(e).(*ast.TypeAssertExpr)
	if !ok {
		return nil
	}
	return ta.X
}
