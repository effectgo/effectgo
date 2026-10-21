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
	"golang.org/x/tools/go/types/typeutil"

	"github.com/effectgo/effectgo/errset"
	"github.com/effectgo/effectgo/internal/dirparse"
)

// Directive marks a named type as must-use.
const Directive = "//effectgo:mustuse"

// keyword is the directive keyword of Directive.
const keyword = "mustuse"

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
	Requires:  []*analysis.Analyzer{errset.Analyzer},
	FactTypes: []analysis.Fact{new(isMustUse), new(errorFree)},
	Run:       run,
}

func init() {
	Analyzer.Flags.StringVar(&allowFlag, "allow", "",
		"comma-separated fully-qualified funcs whose results may be discarded, e.g. os.Remove,(*os.File).Close")
}

var errorIface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

// errorFree is exported on a function whose error set errset proves empty (see
// errset.Result.NoErrors). It carries the result of errset across packages.
type errorFree struct {
	Name string
}

func (*errorFree) AFact()           {}
func (f *errorFree) String() string { return "errorfree(" + f.Name + ")" }

func run(pass *analysis.Pass) (any, error) {
	exportMarkedTypes(pass)
	checkFuncDirectives(pass)
	res, _ := pass.ResultOf[errset.Analyzer].(errset.Result)
	for fn := range res.NoErrors {
		if fn.Pkg() == pass.Pkg && !errset.IsStd(pass.Fset, pass.Pkg) && hasErrorResult(fn.Type().(*types.Signature).Results()) {
			pass.ExportObjectFact(fn, &errorFree{Name: fn.Name()})
		}
	}
	defs := singleDefs(pass)

	allow := map[string]bool{}
	for _, s := range strings.Split(allowFlag, ",") {
		if s = strings.TrimSpace(s); s != "" {
			allow[s] = true
		}
	}

	// The statements are visited as ast.Inspect visits the files, the order that the
	// inspector gave. The inspector is not required: the checker keeps the result of
	// every action of a dependency alive, and one inspector per package was a large
	// part of the standalone run's memory (see docs/journal/17-p7b-body-fields-memory.md).
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			if s, ok := n.(*ast.ExprStmt); ok {
				checkDiscarded(pass, res, defs, allow, s)
			}
			return true
		})
	}
	return nil, nil
}

// checkDiscarded reports a call statement whose result is an error or a must-use
// value that is discarded.
func checkDiscarded(pass *analysis.Pass, res errset.Result, defs map[*types.Var]ast.Expr, allow map[string]bool, stmt *ast.ExprStmt) {
	call, ok := ast.Unparen(stmt.X).(*ast.CallExpr)
	if !ok {
		return
	}
	fn := calleeOf(pass, call, defs)
	if fn != nil && isAllowed(fn, allow) {
		return
	}
	t := pass.TypesInfo.TypeOf(call)
	marked := markedResult(pass, t)
	if !marked {
		// Only an error can be discarded here. A callee whose error set is
		// provably empty returns no error that matters.
		if !hasErrorResult(t) || provablyNoError(pass, res, fn) {
			return
		}
	}
	name := types.ExprString(call.Fun)
	if fn != nil {
		name = fn.Origin().FullName()
	}
	if marked {
		pass.Reportf(call.Pos(), "result of %s is discarded: %s is //effectgo:mustuse; handle it or assign to _", name, markedType(pass, t))
		return
	}
	pass.Reportf(call.Pos(), "result of %s is discarded; handle it or assign to _", name)
}

// calleeOf returns the function that a call invokes: the static callee, or, for
// a call through a local func variable, the function assigned to it when that is
// resolvable (see singleDefs). It returns nil otherwise.
func calleeOf(pass *analysis.Pass, call *ast.CallExpr, defs map[*types.Var]ast.Expr) *types.Func {
	if fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func); ok {
		return fn
	}
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil
	}
	return funcOfExpr(pass, ast.Expr(id), defs, 0)
}

// funcOfExpr resolves an expression that denotes a function: a function name, a
// qualified function, a method value or method expression, or a local variable
// whose single assignment is one of those. depth bounds a chain of variables.
func funcOfExpr(pass *analysis.Pass, e ast.Expr, defs map[*types.Var]ast.Expr, depth int) *types.Func {
	if depth > 8 {
		return nil
	}
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		switch obj := pass.TypesInfo.Uses[e].(type) {
		case *types.Func:
			return obj
		case *types.Var:
			if rhs, ok := defs[obj]; ok {
				return funcOfExpr(pass, rhs, defs, depth+1)
			}
		}
	case *ast.SelectorExpr:
		if sel, ok := pass.TypesInfo.Selections[e]; ok {
			if fn, ok := sel.Obj().(*types.Func); ok {
				return fn
			}
			return nil
		}
		if fn, ok := pass.TypesInfo.Uses[e.Sel].(*types.Func); ok {
			return fn
		}
	}
	return nil
}

// singleDefs maps each local variable of this package that is assigned exactly
// once, by := or =, or declared with one initializer, to the expression assigned
// to it. A variable whose address is taken is left out: it may change through a
// pointer. Function parameters and variables assigned in a range are left out.
func singleDefs(pass *analysis.Pass) map[*types.Var]ast.Expr {
	count := map[*types.Var]int{}
	rhs := map[*types.Var]ast.Expr{}
	addrOf := map[*types.Var]bool{}
	note := func(e ast.Expr, value ast.Expr) {
		id, ok := ast.Unparen(e).(*ast.Ident)
		if !ok || id.Name == "_" {
			return
		}
		v, ok := pass.TypesInfo.ObjectOf(id).(*types.Var)
		if !ok {
			return
		}
		count[v]++
		rhs[v] = value
	}
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					var value ast.Expr
					if len(n.Lhs) == len(n.Rhs) {
						value = n.Rhs[i]
					}
					note(lhs, value)
				}
			case *ast.ValueSpec:
				for i, name := range n.Names {
					var value ast.Expr
					if len(n.Names) == len(n.Values) {
						value = n.Values[i]
					}
					note(name, value)
				}
			case *ast.RangeStmt:
				note(n.Key, nil)
				note(n.Value, nil)
			case *ast.UnaryExpr:
				if id, ok := ast.Unparen(n.X).(*ast.Ident); ok && n.Op == token.AND {
					if v, ok := pass.TypesInfo.ObjectOf(id).(*types.Var); ok {
						addrOf[v] = true
					}
				}
			}
			return true
		})
	}
	out := map[*types.Var]ast.Expr{}
	for v, n := range count {
		if n == 1 && !addrOf[v] && rhs[v] != nil {
			out[v] = rhs[v]
		}
	}
	return out
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
				marks := markComments(ts.Doc)
				if len(gd.Specs) == 1 {
					marks = append(marks, markComments(gd.Doc)...)
				}
				if len(marks) == 0 {
					continue
				}
				obj, _ := pass.TypesInfo.Defs[ts.Name].(*types.TypeName)
				if obj == nil {
					continue
				}
				for _, c := range marks {
					reportArgs(pass, c)
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

// checkFuncDirectives reports the directive on a function declaration. It marks a
// named type, and no analyzer reads it on a function, so on a function it checks
// nothing.
func checkFuncDirectives(pass *analysis.Pass) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || len(markComments(fd.Doc)) == 0 {
				continue
			}
			name := declName(fd)
			if fd.Type.Results.NumFields() == 0 {
				pass.Reportf(fd.Name.Pos(), "effectgo:mustuse on %s checks nothing: it has no results; %s marks a named type", name, Directive)
				continue
			}
			pass.Reportf(fd.Name.Pos(), "effectgo:mustuse on %s checks nothing: %s marks a named type, not a function; put it on the named type that %s returns", name, Directive, name)
		}
	}
}

// declName names a function declaration as the diagnostics show it: F, or T.M for
// a method of T.
func declName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) != 1 {
		return fd.Name.Name
	}
	t := fd.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ix, ok := t.(*ast.IndexExpr); ok {
		t = ix.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

// markComments returns the comments of cg that carry the mustuse keyword. The keyword
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

// reportArgs reports a mark with arguments, such as "//effectgo:mustuse extra". The
// mark still applies: the argument is an error, as it is for pure in effects.
func reportArgs(pass *analysis.Pass, c *ast.Comment) {
	if _, args, _ := dirparse.Parse(c.Text); len(args) > 0 {
		pass.Reportf(c.Pos(), "%s takes no arguments", Directive)
	}
}

func isAllowed(fn *types.Func, allow map[string]bool) bool {
	full := fn.Origin().FullName()
	if allow[full] {
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

// provablyNoError reports whether fn's error set is known to be empty: errset
// proved it in this package, or the defining package exported the fact.
func provablyNoError(pass *analysis.Pass, res errset.Result, fn *types.Func) bool {
	if fn == nil {
		return false
	}
	fn = fn.Origin()
	if res.NoErrors[fn] {
		return true
	}
	if fn.Pkg() == nil || errset.IsStd(pass.Fset, fn.Pkg()) {
		return false // a standard library function is known only through a stub
	}
	var f errorFree
	return pass.ImportObjectFact(fn, &f)
}

// resultsOf returns the result types of a call's type: one, or the components of
// a tuple.
func resultsOf(t types.Type) []types.Type {
	if t == nil {
		return nil
	}
	if tup, ok := t.(*types.Tuple); ok && tup.Len() == 0 {
		return nil
	}
	if tup, ok := t.(*types.Tuple); ok {
		out := make([]types.Type, tup.Len())
		for i := range out {
			out[i] = tup.At(i).Type()
		}
		return out
	}
	return []types.Type{t}
}

// hasErrorResult reports whether a call's results include an error.
func hasErrorResult(t types.Type) bool {
	for _, r := range resultsOf(t) {
		if types.Implements(r, errorIface) {
			return true
		}
	}
	return false
}

// markedResult reports whether a call's results include a must-use named type
// (or a pointer to one).
func markedResult(pass *analysis.Pass, t types.Type) bool {
	for _, r := range resultsOf(t) {
		if typeMarked(pass, r) {
			return true
		}
	}
	return false
}

// markedType names the first must-use result of a call, for the message of a
// discarded value: "*a.Charge", with the package name as qualifier.
func markedType(pass *analysis.Pass, t types.Type) string {
	for _, r := range resultsOf(t) {
		if typeMarked(pass, r) {
			return types.TypeString(r, func(p *types.Package) string { return p.Name() })
		}
	}
	return ""
}

func typeMarked(pass *analysis.Pass, t types.Type) bool {
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
