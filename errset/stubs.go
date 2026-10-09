package errset

// Error stubs. The file format is shared with the effects analyzer and is
// parsed by internal/stubs. This analyzer consumes the "errors" rules; the
// "effects" rules belong to effects and are ignored here.
//
// An errors rule names a function or method and lists its error variants:
//
//	errors <target> <variant,variant,...>   (or: none)
//
// Variants are fully qualified: pkgpath.Name (sentinel), type:pkgpath.Type,
// iface:pkgpath.Iface, or ?. Stubs take precedence over contracts and inference.
//
// A variant whose package is in the import graph of the analyzed package is
// resolved against the type information: a missing name or a kind mismatch is
// a hard error. A variant whose package is not in the import graph is kept as
// its qualified string and matched by string identity; its method set is then
// unknown, so iface: coverage can only match by key.

import (
	"fmt"
	"go/ast"
	"go/types"
	"strconv"
	"strings"

	"github.com/faintghost/effectgo/internal/stubs"
)

// stubToken is one parsed variant of an errors rule.
type stubToken struct {
	kind     string // KindOpaque, KindSentinel, KindType or KindIface
	qual     string // pkgpath.Name, empty for opaque
	pkg, sym string // split of qual
}

// stubRule is one errors rule with its variants parsed.
type stubRule struct {
	file   string
	line   int
	target stubs.Target
	none   bool
	tokens []stubToken
}

func parseVariant(tok string) (stubToken, error) {
	switch {
	case tok == "?":
		return stubToken{kind: KindOpaque}, nil
	case tok == "none":
		return stubToken{}, fmt.Errorf("none must be the whole variant list")
	case strings.HasPrefix(tok, "param:"):
		return stubToken{}, fmt.Errorf("%s: param: variants are not allowed in stub files", tok)
	}
	kind, qual := KindSentinel, tok
	switch {
	case strings.HasPrefix(tok, "type:"):
		kind, qual = KindType, strings.TrimPrefix(tok, "type:")
	case strings.HasPrefix(tok, ifacePrefix):
		kind, qual = KindIface, strings.TrimPrefix(tok, ifacePrefix)
	}
	pkg, sym, err := stubs.SplitQual(qual)
	if err != nil {
		return stubToken{}, fmt.Errorf("variant %v (must be pkgpath.Name, type:pkgpath.Type, iface:pkgpath.Iface or ?)", err)
	}
	return stubToken{kind: kind, qual: qual, pkg: pkg, sym: sym}, nil
}

// parseErrorRules turns the errors rules of a parsed stub file into stubRules.
func parseErrorRules(rules []stubs.Rule) ([]stubRule, error) {
	var out []stubRule
	for _, r := range rules {
		if r.Verb != stubs.VerbErrors {
			continue
		}
		sr := stubRule{file: r.File, line: r.Line, target: r.Target}
		if r.Value == "none" {
			sr.none = true
		} else {
			for _, tok := range strings.Split(r.Value, ",") {
				v, err := parseVariant(tok)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %v", r.File, r.Line, err)
				}
				sr.tokens = append(sr.tokens, v)
			}
		}
		out = append(out, sr)
	}
	return out, nil
}

// parseStubFile parses one stub file and returns its errors rules.
func parseStubFile(path string) ([]stubRule, error) {
	rules, err := stubs.ParseFile(path)
	if err != nil {
		return nil, err
	}
	return parseErrorRules(rules)
}

// loadStubs parses the stub files named by -stubs (or EFFECTGO_STUBS) and
// resolves them against this package's import graph.
func (p *pkgState) loadStubs() error {
	rules, err := stubs.Load(stubs.Paths())
	if err != nil {
		return err
	}
	errRules, err := parseErrorRules(rules)
	if err != nil {
		return err
	}
	var statusRules []statusRule
	p.loadStatusFuncs(rules)
	for _, r := range rules {
		if r.Verb == stubs.VerbStatus && r.Target.Name == "" {
			code, _ := strconv.Atoi(r.Value) // checked by internal/stubs
			statusRules = append(statusRules, statusRule{pkg: r.Target.Pkg, typ: r.Target.Type, code: code})
		}
	}
	p.loadStatusStubs(statusRules)
	p.stubs = map[string]errSet{}
	for _, r := range errRules {
		set := errSet{}
		if !r.none {
			for _, t := range r.tokens {
				v, err := p.stubVariant(t)
				if err != nil {
					return fmt.Errorf("%s:%d: %w", r.file, r.line, err)
				}
				set.put(v)
			}
		}
		p.stubs[p.targetKeyOf(r.target)] = set
	}
	return nil
}

// targetKeyOf is the key under which a stub rule is stored. It matches
// targetKey of the function it names; pointer receivers share the key of the
// value receiver, as targetKey does.
func (p *pkgState) targetKeyOf(t stubs.Target) string {
	if t.TypeOnly {
		return t.Pkg + "." + t.Type // a named func type: the key of its type name
	}
	if !t.Method {
		return t.Pkg + "." + t.Name
	}
	if t.Pkg == "" {
		return t.Type + "." + t.Name // predeclared (error).M
	}
	pkg, typ := p.canonicalType(t.Pkg, t.Type)
	return pkg + "." + typ + "." + t.Name
}

// canonicalType follows a type alias (for example fuego.ContextWithBody) to
// the origin named type, when the package is in the import graph.
func (p *pkgState) canonicalType(pkgPath, name string) (string, string) {
	pkg := p.findPkg(pkgPath)
	if pkg == nil {
		return pkgPath, name
	}
	tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok || !tn.IsAlias() {
		return pkgPath, name
	}
	if n, ok := types.Unalias(tn.Type()).(*types.Named); ok {
		o := n.Origin().Obj()
		if o.Pkg() != nil {
			return o.Pkg().Path(), o.Name()
		}
	}
	return pkgPath, name
}

// findPkg returns the package with the given path from the transitive import
// graph of the analyzed package, or nil.
func (p *pkgState) findPkg(path string) *types.Package {
	if path == p.pass.Pkg.Path() {
		return p.pass.Pkg
	}
	if p.pkgIndex == nil {
		p.pkgIndex = map[string]*types.Package{}
		var walk func(*types.Package)
		walk = func(pkg *types.Package) {
			for _, imp := range pkg.Imports() {
				if _, seen := p.pkgIndex[imp.Path()]; seen {
					continue
				}
				p.pkgIndex[imp.Path()] = imp
				walk(imp)
			}
		}
		walk(p.pass.Pkg)
	}
	return p.pkgIndex[path]
}

// stubVariant resolves one variant of a stub rule.
//
// The package scope of an imported package is partial: export data holds only
// the objects that the analyzed package's dependencies reference. So a name
// missing from an imported package is not an error; the variant falls back to
// string identity. A missing name in the analyzed package itself is an error.
func (p *pkgState) stubVariant(t stubToken) (Variant, error) {
	if t.kind == KindOpaque {
		return opaqueVariant(), nil
	}
	pkg := p.findPkg(t.pkg)
	if pkg == nil {
		return identityVariant(t), nil
	}
	obj := pkg.Scope().Lookup(t.sym)
	if obj == nil {
		if pkg == p.pass.Pkg {
			return Variant{}, fmt.Errorf("%s: no such name in this package", t.qual)
		}
		return identityVariant(t), nil
	}
	v, ok := p.variantFromObject(obj, t.kind)
	if !ok {
		return Variant{}, fmt.Errorf("%s is not a %s", t.qual, map[string]string{
			KindSentinel: "sentinel error variable",
			KindType:     "named error type",
			KindIface:    "named interface",
		}[t.kind])
	}
	return v, nil
}

// identityVariant is a stub variant whose package is not resolved: it is
// compared by its qualified string, and its method set is unknown.
func identityVariant(t stubToken) Variant {
	short := shortPkg(t.pkg) + "." + t.sym
	switch t.kind {
	case KindSentinel:
		return Variant{Key: t.qual, Name: short, Kind: KindSentinel}
	case KindType:
		// Its methods are unknown, so its status is too.
		return Variant{Key: t.qual, Name: short, Kind: KindType, DynamicStatus: true}
	default:
		return Variant{Key: ifacePrefix + t.qual, Name: "some " + short, Kind: KindIface}
	}
}

// shortPkg is the package name a path usually declares: its last element, or
// the element before a major-version suffix (".../pgx/v5" is "pgx"). It is the
// name the resolved variants use (types.Package.Name), so the two agree.
func shortPkg(path string) string {
	elems := strings.Split(path, "/")
	last := elems[len(elems)-1]
	if len(elems) > 1 && isMajorVersion(last) {
		return elems[len(elems)-2]
	}
	return last
}

// isMajorVersion reports whether a path element is a major-version suffix: v2, v10.
func isMajorVersion(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// targetKey is the stub key of a function or method (origin object).
func targetKey(f *types.Func) string {
	if f.Pkg() == nil {
		return ""
	}
	if recv := f.Type().(*types.Signature).Recv(); recv != nil {
		t := types.Unalias(recv.Type())
		if ptr, ok := t.(*types.Pointer); ok {
			t = types.Unalias(ptr.Elem())
		}
		n, ok := t.(*types.Named)
		if !ok {
			return ""
		}
		return objKey(n.Obj()) + "." + f.Name()
	}
	return objKey(f)
}

// stubSet returns the stub's error set for f, if a stub names f.
func (p *pkgState) stubSet(f *types.Func) (errSet, bool) {
	if f == nil || len(p.stubs) == 0 {
		return nil, false
	}
	key := targetKey(f.Origin())
	if key == "" {
		return nil, false
	}
	s, ok := p.stubs[key]
	return s, ok
}

// stubLabel is the witness label of a stub-provided variant.
func stubLabel(f *types.Func) string {
	if f.Type().(*types.Signature).Recv() != nil {
		return "(" + recvTypeName(f) + ")." + f.Name() + " [stub]"
	}
	return objDisplay(f) + " [stub]"
}

// dynamicStubSet is the stub error set of a call through a value that a stub
// names: a package-level variable of func type ("pkgpath.Var"), or a value of
// a named func type ("(pkgpath.Type)"). The variable takes precedence.
func (p *pkgState) dynamicStubSet(info *types.Info, call *ast.CallExpr) (errSet, string, bool) {
	if len(p.stubs) == 0 {
		return nil, "", false
	}
	var id *ast.Ident
	switch f := unparen(call.Fun).(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		if info.Selections[f] == nil { // qualified identifier pkg.Var
			id = f.Sel
		}
	}
	if id != nil {
		if v, ok := info.Uses[id].(*types.Var); ok && v.Pkg() != nil && v.Parent() == v.Pkg().Scope() {
			if s, ok := p.stubs[v.Pkg().Path()+"."+v.Name()]; ok {
				return s, v.Name() + " [stub]", true
			}
		}
	}
	if n, ok := types.Unalias(info.TypeOf(call.Fun)).(*types.Named); ok {
		o := n.Origin().Obj()
		if o.Pkg() != nil {
			if s, ok := p.stubs[o.Pkg().Path()+"."+o.Name()]; ok {
				return s, "(" + o.Name() + ") call [stub]", true
			}
		}
	}
	return nil, "", false
}
