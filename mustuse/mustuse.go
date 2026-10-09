// Package mustuse is the effectgo analyzer for Rust-style must-use results.
//
// It reports a call whose results are discarded by a bare expression statement
// when one of its results is an error, or a named type marked //effectgo:mustuse
// (or a pointer to one). An explicit discard (`_ = f()`), a go statement and a
// defer statement are not reported.
//
// Error results are the concern of errset: a call whose error set is provably
// empty (a function that always returns nil, or a stub rule `errors <target>
// none`) is not reported. A discarded value of a //effectgo:mustuse type is
// always reported.
package mustuse

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/types/typeutil"
)

// Directive marks a named type as must-use.
const Directive = "//effectgo:mustuse"

// isMustUse is exported as an object fact on the *types.TypeName of a marked
// type, so that packages importing the defining package can see the mark.
// gob refuses to encode a struct without exported fields, hence Name.
type isMustUse struct {
	Name string
}

func (*isMustUse) AFact()           {}
func (f *isMustUse) String() string { return "mustuse(" + f.Name + ")" }

// allowFlag holds the raw value of -mustuse.allow.
var allowFlag string

// Analyzer is the mustuse analyzer.
var Analyzer = &analysis.Analyzer{
	Name:      "mustuse",
	Doc:       "report discarded error results and results of //effectgo:mustuse types",
	Requires:  []*analysis.Analyzer{inspect.Analyzer},
	FactTypes: []analysis.Fact{new(isMustUse)},
	Run:       run,
}

func init() {
	Analyzer.Flags.StringVar(&allowFlag, "allow", "",
		"comma-separated fully-qualified funcs whose results may be discarded, e.g. os.Remove,(*os.File).Close")
}

// builtinAllow lists calls whose error results are conventionally ignored.
var builtinAllow = map[string]bool{
	"(*bytes.Buffer).Write":          true,
	"(*bytes.Buffer).WriteString":    true,
	"(*bytes.Buffer).WriteByte":      true,
	"(*bytes.Buffer).WriteRune":      true,
	"(*strings.Builder).Write":       true,
	"(*strings.Builder).WriteString": true,
	"(*strings.Builder).WriteByte":   true,
	"(*strings.Builder).WriteRune":   true,
}

var errorIface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

func run(pass *analysis.Pass) (any, error) {
	exportMarkedTypes(pass)

	allow := map[string]bool{}
	for _, s := range strings.Split(allowFlag, ",") {
		if s = strings.TrimSpace(s); s != "" {
			allow[s] = true
		}
	}

	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	ins.Preorder([]ast.Node{(*ast.ExprStmt)(nil)}, func(n ast.Node) {
		call, ok := ast.Unparen(n.(*ast.ExprStmt).X).(*ast.CallExpr)
		if !ok {
			return
		}
		fn, _ := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
		if fn != nil && isAllowed(fn, allow) {
			return
		}
		if !resultsMustUse(pass, pass.TypesInfo.TypeOf(call)) {
			return
		}
		name := types.ExprString(call.Fun)
		if fn != nil {
			name = fn.FullName()
		}
		pass.Reportf(call.Pos(), "result of %s is discarded; handle it or assign to _", name)
	})
	return nil, nil
}

// exportMarkedTypes exports an isMustUse fact for every marked type declared
// in this package. A directive on an alias is reported: the mark belongs to the
// defined type the alias names.
func exportMarkedTypes(pass *analysis.Pass) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				marked := hasDirective(ts.Doc) || (len(gd.Specs) == 1 && hasDirective(gd.Doc))
				if !marked {
					continue
				}
				obj, _ := pass.TypesInfo.Defs[ts.Name].(*types.TypeName)
				if obj == nil {
					continue
				}
				if obj.IsAlias() {
					pass.Reportf(ts.Name.Pos(), "%s: %s applies to a defined type, not an alias; put it on the type the alias names", ts.Name.Name, Directive)
					continue
				}
				if _, ok := obj.Type().(*types.Named); ok {
					pass.ExportObjectFact(obj, &isMustUse{Name: ts.Name.Name})
				}
			}
		}
	}
}

func hasDirective(cg *ast.CommentGroup) bool {
	if cg == nil {
		return false
	}
	for _, c := range cg.List {
		if strings.TrimSpace(c.Text) == Directive {
			return true
		}
	}
	return false
}

func isAllowed(fn *types.Func, allow map[string]bool) bool {
	full := fn.Origin().FullName()
	if allow[full] || builtinAllow[full] {
		return true
	}
	// fmt.Print*, fmt.Fprint* (package-level functions only).
	sig := fn.Type().(*types.Signature)
	if pkg := fn.Pkg(); pkg != nil && pkg.Path() == "fmt" && sig.Recv() == nil {
		n := fn.Name()
		return strings.HasPrefix(n, "Print") || strings.HasPrefix(n, "Fprint")
	}
	return false
}

// resultsMustUse reports whether any result type of a call is an error or a
// must-use named type (or a pointer to one).
func resultsMustUse(pass *analysis.Pass, t types.Type) bool {
	if t == nil {
		return false
	}
	if tup, ok := t.(*types.Tuple); ok {
		for i := 0; i < tup.Len(); i++ {
			if typeMustUse(pass, tup.At(i).Type()) {
				return true
			}
		}
		return false
	}
	return typeMustUse(pass, t)
}

func typeMustUse(pass *analysis.Pass, t types.Type) bool {
	if types.Implements(t, errorIface) {
		return true
	}
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	var f isMustUse
	return pass.ImportObjectFact(named.Obj(), &f)
}
