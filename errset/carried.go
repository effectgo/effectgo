package errset

// Status-carrying errors. A status rule that names a function or a package
// variable (not a type) says that calling it returns an error that carries an
// HTTP status of its own: huma.Error404NotFound(…) carries 404, and
// huma.NewError(404, …) carries the constant 404 of its first argument.
//
//	status github.com/danielgtaylor/huma/v2.Error404NotFound 404
//	status github.com/danielgtaylor/huma/v2.NewError arg:0
//
// The returned variant is a status-carrying variant (Variant.Carried): it has
// the status of the call, and the methods of the function's result interface,
// so an errmap that passes through the result interface (huma's StatusError)
// keeps the status. Its display names the call, for example huma.Error404NotFound
// or huma.NewError(404). A non-constant status argument gives a variant whose
// status is unknown (DynamicStatus).
//
// This file also builds the default errmap of a framework that has no
// //effectgo:errmap directive (see DefaultErrmap), and the mapper evidence of
// every func(error) error in a package (Result.Mappers).

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"

	"github.com/faintghost/effectgo/internal/stubs"
)

// funcKey is the stub key of a declared function or of a package-level variable:
// pkgpath.Name. It is "" for a function without a package.
func funcKey(fn *types.Func) string {
	if fn.Pkg() == nil {
		return ""
	}
	return fn.Pkg().Path() + "." + fn.Name()
}

// varKey is the stub key of a package-level variable.
func varKey(v *types.Var) string {
	if v.Pkg() == nil {
		return ""
	}
	return v.Pkg().Path() + "." + v.Name()
}

// loadStatusFuncs reads the status rules that name a function or a variable
// (a target without parentheses). Type rules are read by loadStatusStubs.
func (p *pkgState) loadStatusFuncs(rules []stubs.Rule) {
	p.statusFuncs = map[string]stubs.StatusValue{}
	for _, r := range rules {
		if r.Verb != stubs.VerbStatus || r.Target.Name == "" {
			continue
		}
		v, err := stubs.ParseStatusValue(r.Value)
		if err != nil {
			continue // checked by internal/stubs
		}
		p.statusFuncs[r.Target.Pkg+"."+r.Target.Name] = v
	}
}

// resultMethods is the method set of the interface that a function returns as
// its first result, or nil when the first result is not an interface.
func resultMethods(sig *types.Signature) []string {
	if sig == nil || sig.Results().Len() == 0 {
		return nil
	}
	t := sig.Results().At(0).Type()
	if !types.IsInterface(t) {
		return nil
	}
	return methodsOf(t)
}

// carriedVariant builds the variant of a call to a status-function rule. ok is
// false when no rule names the callee.
func (p *pkgState) carriedVariant(key string, sig *types.Signature, display string, call *ast.CallExpr) (Variant, bool) {
	sv, ok := p.statusFuncs[key]
	if !ok {
		return Variant{}, false
	}
	v := Variant{Key: key, Name: display, Kind: KindType, Methods: resultMethods(sig), Carried: true}
	if sv.ArgIdx < 0 {
		v.Status = sv.Code
		return v, true
	}
	code, ok := constArgStatus(p.pass.TypesInfo, call, sv.ArgIdx)
	if !ok {
		v.DynamicStatus = true
		v.Name = display + "(?)"
		return v, true
	}
	v.Status = code
	v.Key = fmt.Sprintf("%s(%d)", key, code)
	v.Name = fmt.Sprintf("%s(%d)", display, code)
	return v, true
}

// constArgStatus returns the constant status argument at index i of call.
func constArgStatus(info *types.Info, call *ast.CallExpr, i int) (int, bool) {
	if call == nil || i >= len(call.Args) {
		return 0, false
	}
	tv, ok := info.Types[call.Args[i]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	v, exact := constant.Int64Val(tv.Value)
	if !exact || v < 100 || v > 599 {
		return 0, false
	}
	return int(v), true
}

// carriedOfCall returns the carried variant of a call to a declared function
// (static callee) that a status rule names.
func (p *pkgState) carriedOfCall(fn *types.Func, call *ast.CallExpr) (Variant, bool) {
	if len(p.statusFuncs) == 0 || fn == nil {
		return Variant{}, false
	}
	sig, _ := fn.Type().(*types.Signature)
	return p.carriedVariant(funcKey(fn), sig, shortDisplay(fn.Pkg(), fn.Name()), call)
}

// carriedOfVarCall returns the carried variant of a call through a package
// variable that a status rule names (huma.NewError).
func (p *pkgState) carriedOfVarCall(call *ast.CallExpr) (Variant, bool) {
	if len(p.statusFuncs) == 0 {
		return Variant{}, false
	}
	var id *ast.Ident
	switch f := unparen(call.Fun).(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		if p.pass.TypesInfo.Selections[f] == nil {
			id = f.Sel
		}
	}
	if id == nil {
		return Variant{}, false
	}
	v, ok := p.pass.TypesInfo.Uses[id].(*types.Var)
	if !ok || v.Pkg() == nil || v.Parent() != v.Pkg().Scope() {
		return Variant{}, false
	}
	sig, _ := v.Type().Underlying().(*types.Signature)
	return p.carriedVariant(varKey(v), sig, shortDisplay(v.Pkg(), v.Name()), call)
}

// callCarriedCode is the constant status of a call that a status rule names,
// for an errmap branch that returns the call (return huma.Error404NotFound(…)).
func (p *pkgState) callCarriedCode(call *ast.CallExpr) (int, bool) {
	if v, ok := p.carriedOfCall(calleeOf(p.pass.TypesInfo, call), call); ok {
		return v.Status, v.Status > 0
	}
	if v, ok := p.carriedOfVarCall(call); ok {
		return v.Status, v.Status > 0
	}
	return 0, false
}

// calleeOf is the static callee of call, or nil.
func calleeOf(info *types.Info, call *ast.CallExpr) *types.Func {
	var fn *types.Func
	switch f := unparen(call.Fun).(type) {
	case *ast.Ident:
		fn, _ = info.Uses[f].(*types.Func)
	case *ast.SelectorExpr:
		fn, _ = info.Uses[f.Sel].(*types.Func)
	}
	if fn == nil {
		return nil
	}
	return fn.Origin()
}

// shortDisplay is "pkgname.Name", the display form of a package-level object.
func shortDisplay(pkg *types.Package, name string) string {
	if pkg == nil {
		return name
	}
	return pkg.Name() + "." + name
}

// DefaultErrmap is the errmap of a framework that maps errors by one interface
// and sends everything else as a fixed status. An error whose type implements
// the interface keeps the status it carries (a carried variant, see carried.go);
// a value that implements it without a carried status has an unknown status,
// because the framework reads the status from a method this analyzer does not
// evaluate; every other error gets fallback.
//
// iface is the interface's qualified name (pkgpath.Name), methods its method set
// as sigKey strings (see methodsOf), and fallback the status of the rest.
func DefaultErrmap(iface string, methods []string, fallback int) *ErrmapFact {
	return &ErrmapFact{Branches: []Branch{
		{Match: matchIface, Key: iface, Methods: methods, Passthrough: true, Carrier: true},
		{Match: matchAny, Status: fallback},
	}}
}

// IfaceMethods returns the method set of the named interface pkgpath.Name in
// the import graph of pkg (pkg itself included), in methodsOf form. ok is false
// when the interface is not found.
func IfaceMethods(pkg *types.Package, qualified string) ([]string, bool) {
	ipath, name, err := stubs.SplitQual(qualified)
	if err != nil {
		return nil, false
	}
	tn, ok := findPkgObject(pkg, map[string]bool{}, ipath, name).(*types.TypeName)
	if !ok || !types.IsInterface(tn.Type()) {
		return nil, false
	}
	return methodsOf(tn.Type()), true
}

// findPkgObject searches the import graph of pkg for the object name in the
// package whose path is pkgPath.
func findPkgObject(pkg *types.Package, seen map[string]bool, pkgPath, name string) types.Object {
	if pkg == nil || seen[pkg.Path()] {
		return nil
	}
	seen[pkg.Path()] = true
	if pkg.Path() == pkgPath {
		return pkg.Scope().Lookup(name)
	}
	for _, imp := range pkg.Imports() {
		if o := findPkgObject(imp, seen, pkgPath, name); o != nil {
			return o
		}
	}
	return nil
}

// Mapping is one piece of evidence that an errmap or a mapper function maps a
// variant to a status: errors.Is(err, S) (or a case, or errors.As on a concrete
// type) returns a constant status.
type Mapping struct {
	Key    string // variant key
	Status int
}

// MappingsOf lists the sentinel and type branches of an errmap that carry a
// known status. An interface branch is not a mapping of one variant.
func MappingsOf(m *ErrmapFact) []Mapping {
	var out []Mapping
	for _, b := range m.Branches {
		if b.Status <= 0 || b.Passthrough || (b.Match != matchSentinel && b.Match != matchType) {
			continue
		}
		out = append(out, Mapping{Key: b.Key, Status: b.Status})
	}
	return out
}

// isErrorMapper reports whether sig is func(error) error.
func isErrorMapper(sig *types.Signature) bool {
	return sig.Params().Len() == 1 && sig.Results().Len() == 1 &&
		types.Identical(sig.Params().At(0).Type(), errorType) &&
		types.Identical(sig.Results().At(0).Type(), errorType)
}

// mappers returns the branches of every func(error) error of the package that
// has a body (Result.Mappers). Directive errmaps are included, so the evidence
// does not depend on the directive.
func (p *pkgState) mappers() map[*types.Func]*ErrmapFact {
	out := map[*types.Func]*ErrmapFact{}
	for _, u := range p.units {
		if isErrorMapper(u.sig) {
			out[u.obj] = p.buildErrmap(u)
		}
	}
	return out
}

// CollapseForeign returns the variants as the analyzed module can see them (see
// collapseForeign). It is the boundary that the report uses; a consumer of
// ErrFact facts applies it to get the same identities.
func CollapseForeign(vs []Variant, module string) []Variant {
	return collapseForeign(vs, module)
}
