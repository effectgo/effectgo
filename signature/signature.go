// Package signature prints the inferred Effect-TS-style signature of every
// declared function of a package:
//
//	Effect<A, E, R>
//
// A is the non-error result type (void when there is none, a tuple when there
// are several). E is the inferred error set from the errset analyzer, or never.
// R is the inferred effect set from the effects analyzer, with placeholders
// for calls through parameters (param:i). For a function that returns a handler
// literal, the signature describes the handler: A is the handler's result and E
// is the handler set.
//
// The analyzer is not part of the default effectgo run. It is reached through
// the "sig" subcommand (see cmd/effectgo), which runs only this analyzer. Its
// two dependencies run as required analyzers, and their own diagnostics are not
// printed, because only the analyzers named on the command line report.
package signature

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/faintghost/effectgo/effects"
	"github.com/faintghost/effectgo/errset"
)

// Analyzer prints one Effect<A, E, R> diagnostic per declared function.
var Analyzer = &analysis.Analyzer{
	Name:     "signature",
	Doc:      "prints the inferred signature Effect<A, E, R> of every declared function",
	Requires: []*analysis.Analyzer{effects.Analyzer, errset.Analyzer},
	Run:      run,
}

var errorIface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

func run(pass *analysis.Pass) (any, error) {
	effRes, _ := pass.ResultOf[effects.Analyzer].(effects.Result)
	errRes, _ := pass.ResultOf[errset.Analyzer].(errset.Result)
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			obj, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func)
			if !ok {
				continue
			}
			// go test compiles a generated driver (_testmain.go) that lives in the
			// build cache under a name without the .go suffix. It is not source.
			if !strings.HasSuffix(pass.Fset.Position(fd.Name.Pos()).Filename, ".go") {
				continue
			}
			errs := errRes.Funcs[obj]
			a := aType(pass, fd, obj, errs.Handler)
			e := eType(pass, errs.Variants)
			r := rType(effRes[obj])
			pass.Reportf(fd.Name.Pos(), "%-15s Effect<%s, %s, %s>", funcName(obj)+":", a, e, r)
		}
	}
	return nil, nil
}

// aType renders the non-error results of fn, or of the handler it returns.
func aType(pass *analysis.Pass, fd *ast.FuncDecl, fn *types.Func, handler bool) string {
	sig := fn.Type().(*types.Signature)
	if handler && sig.Results().Len() == 1 {
		if hs, ok := sig.Results().At(0).Type().Underlying().(*types.Signature); ok {
			sig = hs
		}
	}
	qual := func(p *types.Package) string {
		if p == pass.Pkg {
			return ""
		}
		return p.Name()
	}
	var parts []string
	for i := 0; i < sig.Results().Len(); i++ {
		t := sig.Results().At(i).Type()
		if implementsError(t) {
			continue
		}
		parts = append(parts, types.TypeString(t, qual))
	}
	switch len(parts) {
	case 0:
		return "void"
	case 1:
		return parts[0]
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// eType renders an error set: "never" when empty, otherwise the names joined
// by " | ", sorted, with the opaque "?" last.
func eType(pass *analysis.Pass, vs []errset.Variant) string {
	if len(vs) == 0 {
		return "never"
	}
	names := make([]string, 0, len(vs))
	opaque := false
	for _, v := range vs {
		if v.Kind == errset.KindOpaque {
			opaque = true
			continue
		}
		names = append(names, variantName(pass, v))
	}
	sort.Strings(names)
	if opaque {
		names = append(names, "?")
	}
	return strings.Join(names, " | ")
}

// variantName is the display name of a variant. Names of the analyzed package
// are shown unqualified.
func variantName(pass *analysis.Pass, v errset.Variant) string {
	if v.Kind == errset.KindSentinel || v.Kind == errset.KindType {
		if rest, ok := strings.CutPrefix(v.Key, pass.Pkg.Path()+"."); ok {
			return rest
		}
	}
	return v.Name
}

// rType renders an effect set as "{a, b}", or "{}" when empty. Placeholders
// are included.
func rType(effs []string) string {
	return "{" + strings.Join(effs, ", ") + "}"
}

// funcName is the name used in the signature: "Func", or "Recv.Method" for a
// method, without the pointer star.
func funcName(fn *types.Func) string {
	if recv := fn.Type().(*types.Signature).Recv(); recv != nil {
		t := types.Unalias(recv.Type())
		if p, ok := t.(*types.Pointer); ok {
			t = types.Unalias(p.Elem())
		}
		if n, ok := t.(*types.Named); ok {
			return n.Obj().Name() + "." + fn.Name()
		}
	}
	return fn.Name()
}

// implementsError reports whether t is the error interface or a type that
// implements it, by value or through a pointer. This is the notion of error
// result that the errset analyzer uses.
func implementsError(t types.Type) bool {
	return types.Implements(t, errorIface) || types.Implements(types.NewPointer(t), errorIface)
}
