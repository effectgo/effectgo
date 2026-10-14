// Package schematag is the effectgo analyzer for the schema tags and the route
// inputs of the types that rt/web and rt/schema read.
//
// The types it checks are the ones that flow into rt: the input and output type
// arguments of web.Get, Post, Put, Patch and Delete, and the type argument of
// schema.Decode and schema.Compile, with every struct, slice element, map value
// and pointer reachable from them. A type that never reaches rt is not checked,
// so a `schema` tag of another library (gorilla/schema) is not a finding.
//
// The checks mirror what rt does at registration and at decode time: the rule
// vocabulary and its kinds (vocab.go, read from rt/schema by the test), the field
// types schema supports, the path and query tags of the input (rt/web/input.go),
// the body rule of GET and DELETE, and the enum contract (schema.Enum). Anything
// that rt would panic on at startup is reported here, at vet time.
//
// A call in a _test.go file is not checked: a test registers bad routes on
// purpose, to see the framework panic.
//
// Silence when unproved: a pattern that is not a constant is not checked against
// the path fields, and a pattern name is reported only when no RegisterPattern
// call with a dynamic name is visible.
package schematag

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/faintghost/effectgo/exhaustive"
	"github.com/faintghost/effectgo/internal/rtshape"
)

const (
	webPath    = "github.com/faintghost/effectgo/rt/web"
	schemaPath = "github.com/faintghost/effectgo/rt/schema"
)

// Analyzer is the schematag analyzer.
var Analyzer = &analysis.Analyzer{
	Name:      "schematag",
	Doc:       "check the schema tags, field types, path and query tags and enum Valid methods of the types that rt/web and rt/schema read",
	FactTypes: []analysis.Fact{new(sealedEnumFact)},
	Run:       run,
}

// sealedEnumFact is exported on a sealed enum (exhaustive.SealedEnums) so that
// a package that brings the enum into rt can check it. Analyzers do not share
// facts within a package, so schematag exports its own.
type sealedEnumFact struct{}

func (*sealedEnumFact) AFact()         {}
func (*sealedEnumFact) String() string { return "sealed enum" }

// isSealedEnum reports whether tn is a sealed enum of this package or of an
// imported one.
func isSealedEnum(pass *analysis.Pass, tn *types.TypeName) bool {
	return pass.ImportObjectFact(tn, &sealedEnumFact{})
}

// checker holds the state of one package.
type checker struct {
	pass     *analysis.Pass
	reported map[string]bool
}

func run(pass *analysis.Pass) (any, error) {
	for _, tn := range exhaustive.SealedEnums(pass) {
		pass.ExportObjectFact(tn, &sealedEnumFact{})
	}
	c := &checker{pass: pass, reported: map[string]bool{}}
	for _, f := range pass.Files {
		if isTestFile(pass, f.Pos()) {
			continue // a test registers bad routes on purpose, to see the framework panic
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				c.entry(call)
			}
			return true
		})
	}
	return nil, nil
}

// isTestFile reports whether pos lies in a _test.go file. A call there is a
// fixture of the test (errset exempts a closure in a test for the same reason).
func isTestFile(pass *analysis.Pass, pos token.Pos) bool {
	return strings.HasSuffix(pass.Fset.Position(pos).Filename, "_test.go")
}

// calleeOf returns the origin of the function a call names, and the type
// arguments of its instantiation (nil when it is not generic or not inferred).
func calleeOf(pass *analysis.Pass, call *ast.CallExpr) (*types.Func, []types.Type) {
	fun := ast.Unparen(call.Fun)
	switch x := fun.(type) {
	case *ast.IndexExpr:
		fun = ast.Unparen(x.X)
	case *ast.IndexListExpr:
		fun = ast.Unparen(x.X)
	default: // any other callee is used as it is
	}
	var id *ast.Ident
	switch x := fun.(type) {
	case *ast.Ident:
		id = x
	case *ast.SelectorExpr:
		id = x.Sel
	default:
		return nil, nil
	}
	fn, _ := pass.TypesInfo.Uses[id].(*types.Func)
	if fn == nil {
		return nil, nil
	}
	var targs []types.Type
	if inst, ok := pass.TypesInfo.Instances[id]; ok {
		for i := range inst.TypeArgs.Len() {
			targs = append(targs, inst.TypeArgs.At(i))
		}
	}
	return fn.Origin(), targs
}

// isSchemaFunc reports whether fn is the rt/schema function of that name.
func isSchemaFunc(fn *types.Func, name string) bool {
	return fn.Pkg() != nil && fn.Pkg().Path() == schemaPath && fn.Name() == name
}

// webMethods are the route functions of rt/web and the method each one serves.
var webMethods = map[string]string{"Get": "GET", "Post": "POST", "Put": "PUT", "Patch": "PATCH", "Delete": "DELETE"}

// entry checks a call that brings a type into rt.
func (c *checker) entry(call *ast.CallExpr) {
	fn, targs := calleeOf(c.pass, call)
	if fn == nil || fn.Pkg() == nil {
		return
	}
	switch fn.Pkg().Path() {
	case webPath:
		method, ok := webMethods[fn.Name()]
		if !ok || len(targs) != 2 || len(call.Args) < 2 {
			return
		}
		c.route(call, method, targs[0], targs[1])
	case schemaPath:
		if (fn.Name() == "Decode" || fn.Name() == "Compile") && len(targs) == 1 {
			label := fmt.Sprintf("schema.%s[%s]", fn.Name(), c.typeString(targs[0]))
			c.schemaRoot(call.Pos(), label, targs[0])
		}
	}
}

// route checks one route: its input (path, query and body fields against the
// pattern and the method) and its output (the schema of the output type).
func (c *checker) route(call *ast.CallExpr, method string, in, out types.Type) {
	pattern, isConst := c.constString(call.Args[1])
	text := method + " " + types.ExprString(call.Args[1])
	if isConst {
		text = method + " " + pattern
	}
	c.checkInput(call.Pos(), text, method, pattern, isConst, in)
	c.schemaRoot(call.Pos(), "route "+text+": output "+c.typeString(out), out)
}

// checkInput mirrors rt/web newInputPlan.
func (c *checker) checkInput(pos token.Pos, route, method, pattern string, patConst bool, in types.Type) {
	if _, ok := in.(*types.TypeParam); ok {
		return
	}
	if isNamed(in, webPath, "NoInput") {
		if patConst {
			if w := nameOfPattern(pattern); len(w) > 0 {
				c.routeProblem(pos, route, fmt.Sprintf("pattern has {%s}, but the input is web.NoInput; take the value into an input struct with a path tag, or remove the wildcard", w[0]))
			}
		}
		return
	}
	st, ok := in.Underlying().(*types.Struct)
	if !ok {
		c.routeProblem(pos, route, fmt.Sprintf("input %s must be a struct or web.NoInput", c.typeString(in)))
		return
	}
	c.schemaRoot(pos, "route "+route+": input "+c.typeString(in), in)

	readsBody := method == "POST" || method == "PUT" || method == "PATCH"
	var wild []string
	if patConst {
		wild = nameOfPattern(pattern)
	}
	used := map[string]bool{}
	var bodyField *types.Var
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		tag := reflect.StructTag(st.Tag(i))
		jsonTag := tag.Get("json")
		pathName, hasPath := tag.Lookup("path")
		queryName, hasQuery := tag.Lookup("query")
		if !hasPath && !hasQuery {
			if jsonTag != "-" && bodyField == nil {
				bodyField = f
			}
			continue
		}
		where := "field " + f.Name()
		switch {
		case hasPath && hasQuery:
			c.routeProblem(pos, route, where+" has both path and query tags; keep one")
			continue
		case jsonTag == "-":
			c.routeProblem(pos, route, where+` has a path or query tag but json:"-"; remove the json "-"`)
			continue
		}
		if hasPath {
			if patConst && !contains(wild, pathName) {
				c.routeProblem(pos, route, fmt.Sprintf("%s has path:%q, but the pattern has no {%s}; add {%s} to the pattern or change the tag", where, pathName, pathName, pathName))
			}
			if !scalarType(f.Type()) {
				c.routeProblem(pos, route, fmt.Sprintf("path field %s must be a scalar (string, number, bool or time.Time), got %s", f.Name(), c.typeString(f.Type())))
			}
			used[pathName] = true
			continue
		}
		if queryName == "" {
			c.routeProblem(pos, route, where+" has an empty query name; write query:\"name\"")
			continue
		}
		if !scalarType(f.Type()) && !sliceOfScalar(f.Type()) {
			c.routeProblem(pos, route, fmt.Sprintf("query field %s must be a scalar or a slice of scalars, got %s", f.Name(), c.typeString(f.Type())))
		}
	}
	if patConst {
		for _, w := range wild {
			if !used[w] {
				c.routeProblem(pos, route, fmt.Sprintf("pattern {%s} has no path field; add a field with path:%q", w, w))
			}
		}
	}
	if bodyField != nil && !readsBody {
		c.routeProblem(pos, route, fmt.Sprintf("%s takes no body, but field %s has no path or query tag; read it with query:\"name\", or use POST, PUT or PATCH", method, bodyField.Name()))
	}
}

// schemaRoot checks the schema of a type that is brought into rt at pos.
func (c *checker) schemaRoot(pos token.Pos, label string, t types.Type) {
	if _, ok := t.(*types.TypeParam); ok {
		return
	}
	shape := rtshape.Describe(t, rtshape.Options{Qualifier: c.qualifier(), Sealed: c.sealedMessage})
	s := site{pos: pos, text: label}
	for _, p := range shape.Problems {
		c.problem(p, s)
	}
}

// problem reports one problem of a type. A problem of a field declared in this
// package is reported at the field; any other is reported at the call that
// brings the type into rt, with the field's position.
func (c *checker) problem(p rtshape.Problem, s site) {
	full := p.Msg
	if p.Where != "" {
		full = p.Where + ": " + p.Msg
	}
	if p.Owner != nil && p.Owner.Pkg() == c.pass.Pkg {
		c.report(p.Owner.Pos(), full)
		return
	}
	if p.Owner != nil {
		full += fmt.Sprintf(" (field declared at %s)", c.pass.Fset.Position(p.Owner.Pos()))
	}
	c.report(s.pos, s.text+": "+full)
}

// sealedMessage is the diagnostic for a sealed enum with no Valid method: the
// sealed rule is schematag's, the walk in rtshape only asks for it.
func (c *checker) sealedMessage(n *types.Named) string {
	if !isSealedEnum(c.pass, n.Obj()) || rtshape.HasValid(n) {
		return ""
	}
	return fmt.Sprintf("type %s is a sealed enum with no Valid() bool method, so Decode cannot limit its values; add func (%s) Valid() bool that reports whether the value is one of its constants (schema.Enum)", c.typeString(n), c.typeString(n))
}

// routeProblem reports a route-level problem at the route call.
func (c *checker) routeProblem(pos token.Pos, route, msg string) {
	c.report(pos, "route "+route+": "+msg)
}

// report reports a diagnostic once per position and text.
func (c *checker) report(pos token.Pos, msg string) {
	key := fmt.Sprintf("%d|%s", pos, msg)
	if c.reported[key] {
		return
	}
	c.reported[key] = true
	c.pass.Reportf(pos, "%s", msg)
}

// constString returns the value of a constant string expression.
func (c *checker) constString(e ast.Expr) (string, bool) {
	tv, ok := c.pass.TypesInfo.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// qualifier names a type: unqualified in this package, by package name in others.
func (c *checker) qualifier() types.Qualifier {
	return func(p *types.Package) string {
		if p == c.pass.Pkg {
			return ""
		}
		return p.Name()
	}
}

// typeString names a type with qualifier.
func (c *checker) typeString(t types.Type) string {
	return types.TypeString(t, c.qualifier())
}

// site is where a problem of a type is reported when the field that has it is
// declared in another package: the call that brings the type into rt.
type site struct {
	pos  token.Pos
	text string
}

// scalarType reports whether a field of type t is read from one string: a bool,
// a number, a string or a time, behind any number of pointers (rt/web scalar).
func scalarType(t types.Type) bool {
	for {
		p, ok := t.Underlying().(*types.Pointer)
		if !ok {
			break
		}
		t = p.Elem()
	}
	if isNamed(t, "time", "Time") {
		return true
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch rtshape.BasicKind(b) {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// sliceOfScalar reports whether t is a slice whose elements are scalars.
func sliceOfScalar(t types.Type) bool {
	s, ok := t.Underlying().(*types.Slice)
	return ok && scalarType(s.Elem())
}

// isNamed reports whether t is the named type pkgPath.name.
func isNamed(t types.Type, pkgPath, name string) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == pkgPath && n.Obj().Name() == name
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// nameOfPattern lists the wildcard names of a route pattern, without "..." and
// without the anonymous "$" (rt/web nameOfPattern).
func nameOfPattern(pattern string) []string {
	var names []string
	for _, seg := range strings.Split(pattern, "/") {
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") {
			continue
		}
		name := strings.TrimSuffix(seg[1:len(seg)-1], "...")
		if name != "$" {
			names = append(names, name)
		}
	}
	return names
}
