// Package ginhttp checks the HTTP responses of Gin handlers against the error
// sets that errset infers. A Gin handler returns nothing: it writes the response
// itself, through the gin.Context writers (c.JSON, c.AbortWithStatusJSON, ...)
// or through a project helper that does. So the boundary is not a returned
// error but a call: the analysis reads the constant status of each response
// call (a "sink", see the sink stub verb) and the errors that reach it.
//
// Three things are found per package:
//
//   - sink mappers: a function with a *gin.Context parameter and an error
//     parameter whose body answers with constant statuses inside errors.Is,
//     errors.As, == or switch branches. Its branches (variant -> status) and its
//     default status are exported as a fact, so a handler in another package can
//     use them. Status mappers, func(error) (int, bool) whose errors.Is branches
//     return constant statuses, are exported the same way.
//   - handler findings: for every function or func literal with a *gin.Context
//     parameter, the error variables that reach a 5xx response, through a sink
//     mapper call, an inline "if err != nil { sink(c, N) }" branch, an
//     "if errors.Is(err, X) { sink(c, 4xx) } else { sink(c, 5xx) }" chain, or the
//     "if status, ok := m(err); ok { sink(c, status) }" shape.
//   - leaks: a sink whose body argument carries err.Error() or an error
//     formatted into a string.
//
// The sets come from errset's flow of the enclosing declaration, so they are
// flow-insensitive: see docs/design/limitations.md (Gin adapter) for the limitations.
package ginhttp

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
)

// ginPkg is the import path of the framework.
const ginPkg = "github.com/gin-gonic/gin"

// Finding is one response (or leak) that the analysis found in a package.
type Finding struct {
	Kind       string // "reach" (an error variant reaches the sink) or "leak"
	Handler    string // "func literal (file:line)" or "Name (file:line)"
	HandlerPos string // absolute file:line of the handler; joins routes to findings
	Pos        string // absolute file:line of the sink call
	Variant    string // display name of the variant, or the error expression of a leak
	Key        string // variant key ("pkgpath.Name"); "" for ?, new@ and params
	ErrKind    string // errset kind of the variant: sentinel, type, opaque, fresh, param, iface, or "leak"
	Status     int    // HTTP status of the response; 0 when unknown
	Via        string // witness chain, and how the status was assigned
	Project    bool   // the variant belongs to the analyzed module (reach findings)
}

// Obs is one explicit mapping of a named variant to a status: a mapper branch or
// an inline branch. The driver uses them to tell an inconsistent mapping apart.
type Obs struct {
	Key    string
	Status int
	Pos    string
}

// MapperInfo describes one detected mapper, for the -mappers listing.
type MapperInfo struct {
	Kind       string // "sink" or "status"
	Name       string // function name with its position
	Branches   []string
	Default    int  // the default status of a sink mapper; 0 when unknown or none
	HasDefault bool // a default status exists
}

// Route is one registered Gin route whose handler could be resolved.
type Route struct {
	Method     string
	Path       string // "" when the path is not a constant
	HandlerPos string
	Pos        string
}

// Result is the ResultType of the analyzer.
type Result struct {
	Findings []Finding
	Obs      []Obs
	Mappers  []MapperInfo
	Routes   []Route
	Handlers int // functions and literals with a *gin.Context parameter
}

// sinkMapperFact is exported on a sink mapper.
type sinkMapperFact struct {
	Branches   []mapBranch
	Default    int
	HasDefault bool
	Ctx, Err   int // parameter indexes of the *gin.Context and the error
}

// AFact marks sinkMapperFact as an analysis fact.
func (*sinkMapperFact) AFact() {}

func (f *sinkMapperFact) String() string { return "gin-sink-mapper" }

// statusMapperFact is exported on a func(error) (int, bool) status mapper.
type statusMapperFact struct {
	Branches []mapBranch
}

// AFact marks statusMapperFact as an analysis fact.
func (*statusMapperFact) AFact() {}

func (f *statusMapperFact) String() string { return "gin-status-mapper" }

// factoryFact is exported on a function that returns a handler literal.
type factoryFact struct {
	LitPos string
}

// AFact marks factoryFact as an analysis fact.
func (*factoryFact) AFact() {}

func (f *factoryFact) String() string { return "gin-handler-factory: " + f.LitPos }

// mapBranch is one variant rule of a mapper: the first matching rule decides.
type mapBranch struct {
	Br     errset.Branch
	Status int
	Pos    string
}

// Analyzer is the gin adapter's analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "ginhttp",
	Doc:      "checks the status of Gin responses (sink calls) against the error sets of handlers, and reports error text in responses",
	Requires: []*analysis.Analyzer{errset.Analyzer},
	Run:      run,
	FactTypes: []analysis.Fact{
		new(sinkMapperFact), new(statusMapperFact), new(factoryFact),
	},
	ResultType: reflect.TypeOf(Result{}),
}

// sinkRule is one "sink" stub rule: the argument index of the status, and of
// the body, or -1.
type sinkRule struct {
	status int
	body   int
}

// analyzer holds the state of one package run.
type analyzer struct {
	pass     *analysis.Pass
	info     *types.Info
	errRes   errset.Result
	sinks    map[string]sinkRule
	local    map[*types.Func]*sinkMapperFact
	localS   map[*types.Func]*statusMapperFact
	localF   map[*types.Func]*factoryFact
	groups   map[types.Object]groupDef
	res      Result
	seen     map[string]bool
	module   string
	findings []Finding // reach and leak findings, before dedupe
}

// groupDef is a router group variable: g := base.Group("prefix").
type groupDef struct {
	base   ast.Expr
	prefix string
}

// loadSinks reads the sink rules of the stub files of the pass's package.
func loadSinks(pass *analysis.Pass) (map[string]sinkRule, error) {
	rules, err := stubs.RulesForPass(pass)
	if err != nil {
		return nil, err
	}
	out := map[string]sinkRule{}
	for _, r := range rules {
		if r.Verb != stubs.VerbSink {
			continue
		}
		st, _ := strconv.Atoi(r.Args[0])
		body := -1
		if r.Args[1] != "-1" {
			body, _ = strconv.Atoi(r.Args[1])
		}
		out[r.Target.String()] = sinkRule{status: st, body: body}
	}
	return out, nil
}

func run(pass *analysis.Pass) (any, error) {
	sinks, err := loadSinks(pass)
	if err != nil {
		return nil, err
	}
	a := &analyzer{
		pass:   pass,
		info:   pass.TypesInfo,
		errRes: pass.ResultOf[errset.Analyzer].(errset.Result),
		sinks:  sinks,
		local:  map[*types.Func]*sinkMapperFact{},
		localS: map[*types.Func]*statusMapperFact{},
		localF: map[*types.Func]*factoryFact{},
		groups: map[types.Object]groupDef{},
		seen:   map[string]bool{},
	}
	if pass.Module != nil {
		a.module = pass.Module.Path
	}
	if err := a.run(); err != nil {
		return nil, err
	}
	return a.res, nil
}

// run is the body of the analyzer; the package's facts are exported here.
func (a *analyzer) run() error {
	// Pass 1: the declarations that other declarations depend on.
	var decls []*ast.FuncDecl
	for _, f := range a.pass.Files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				decls = append(decls, fd)
			}
		}
	}
	for _, fd := range decls {
		fn, ok := a.info.Defs[fd.Name].(*types.Func)
		if !ok {
			continue
		}
		if m := a.sinkMapperOf(fd, fn); m != nil {
			a.local[fn] = m
			a.pass.ExportObjectFact(fn, m)
			a.res.Mappers = append(a.res.Mappers, a.mapperInfo("sink", fd, fn, m.Branches, m.Default, m.HasDefault))
		}
		if s := a.statusMapperOf(fd); s != nil {
			a.localS[fn] = s
			a.pass.ExportObjectFact(fn, s)
			a.res.Mappers = append(a.res.Mappers, a.mapperInfo("status", fd, fn, s.Branches, 0, false))
			for _, b := range s.Branches {
				a.res.Obs = append(a.res.Obs, branchObs(b))
			}
		}
		if lit := a.factoryLit(fd); lit != "" {
			f := &factoryFact{LitPos: lit}
			a.localF[fn] = f
			a.pass.ExportObjectFact(fn, f)
		}
	}
	// Pass 2: router groups, so that route prefixes resolve.
	a.collectGroups()
	// Pass 3: handlers. Every declaration and every literal with a *gin.Context
	// parameter is a handler; a declaration's flow covers its nested literals.
	for _, f := range a.pass.Files {
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				if x.Body != nil {
					a.handlersIn(x)
				}
			case *ast.GenDecl:
				a.literalsIn(x, errset.Flow{}, "a package-level variable")
			}
		}
	}
	// Pass 4: leaks and routes over the whole package (declarations and the
	// package-level literals alike).
	for _, f := range a.pass.Files {
		for _, d := range f.Decls {
			a.leaksIn(d)
			a.routesIn(d)
		}
	}
	a.res.Findings = dedupe(a.findings)
	a.res.Obs = dedupeObs(a.res.Obs)
	sort.SliceStable(a.res.Findings, func(i, j int) bool { return a.res.Findings[i].Pos < a.res.Findings[j].Pos })
	return nil
}

// posStr is the absolute file:line of a position.
func (a *analyzer) posStr(p token.Pos) string {
	pos := a.pass.Fset.Position(p)
	return filepath.Clean(pos.Filename) + ":" + strconv.Itoa(pos.Line)
}

// keyOf is the stub target key of a function or method: the same string that
// stubs.Target.String gives for the rule that names it.
func keyOf(f *types.Func) string {
	if f == nil || f.Pkg() == nil {
		return ""
	}
	t := stubs.Target{Pkg: f.Pkg().Path(), Name: f.Name()}
	if recv := f.Type().(*types.Signature).Recv(); recv != nil {
		rt := recv.Type()
		if p, ok := rt.(*types.Pointer); ok {
			t.Ptr = true
			rt = p.Elem()
		}
		n, ok := types.Unalias(rt).(*types.Named)
		if !ok || n.Obj().Pkg() == nil {
			return ""
		}
		t.Method = true
		t.Type = n.Obj().Name()
		t.Pkg = n.Obj().Pkg().Path()
	}
	return t.String()
}

// sinkOf returns the sink rule of a call, if the callee is a sink.
func (a *analyzer) sinkOf(call *ast.CallExpr) (sinkRule, bool) {
	f := calleeOf(a.info, call)
	if f == nil {
		return sinkRule{}, false
	}
	r, ok := a.sinks[keyOf(f)]
	if !ok || r.status >= len(call.Args) || r.body >= len(call.Args) {
		return sinkRule{}, false
	}
	return r, true
}

// statusOfCall returns the constant status argument of a sink call; 0 when it is
// not a constant.
func (a *analyzer) statusOfCall(call *ast.CallExpr, r sinkRule) int {
	n, ok := constInt(a.info, call.Args[r.status])
	if !ok {
		return 0
	}
	return n
}

// calleeOf is the static callee of a call, or nil.
func calleeOf(info *types.Info, call *ast.CallExpr) *types.Func {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		f, _ := info.Uses[fun].(*types.Func)
		return f
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[fun]; ok {
			f, _ := sel.Obj().(*types.Func)
			return f
		}
		f, _ := info.Uses[fun.Sel].(*types.Func)
		return f
	case *ast.IndexExpr:
		return calleeOf(info, &ast.CallExpr{Fun: fun.X})
	}
	return nil
}

// constInt is the value of a constant integer expression.
func constInt(info *types.Info, e ast.Expr) (int, bool) {
	tv, ok := info.Types[e]
	if !ok || tv.Value == nil {
		return 0, false
	}
	v, exact := constant.Int64Val(constant.ToInt(tv.Value))
	return int(v), exact
}

// isGinContext reports whether t is *gin.Context.
func isGinContext(t types.Type) bool {
	p, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return false
	}
	n, ok := types.Unalias(p.Elem()).(*types.Named)
	return ok && n.Obj().Name() == "Context" && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == ginPkg
}

// paramObjs maps the parameters of a function type to their objects, in order.
// Unnamed parameters have no object and are nil.
func paramObjs(info *types.Info, ft *ast.FuncType) []types.Object {
	var out []types.Object
	if ft == nil || ft.Params == nil {
		return nil
	}
	for _, f := range ft.Params.List {
		if len(f.Names) == 0 {
			out = append(out, nil)
			continue
		}
		for _, n := range f.Names {
			out = append(out, info.Defs[n])
		}
	}
	return out
}

// ginCtxIndex returns the index of the *gin.Context parameter in ft, or -1.
func ginCtxIndex(info *types.Info, ft *ast.FuncType) int {
	if ft == nil || ft.Params == nil {
		return -1
	}
	i := 0
	for _, f := range ft.Params.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		if isGinContext(info.TypeOf(f.Type)) {
			return i
		}
		i += n
	}
	return -1
}

// errIndexOf returns the index of the first error-typed parameter in ft, or -1.
func errIndexOf(info *types.Info, ft *ast.FuncType) int {
	if ft == nil || ft.Params == nil {
		return -1
	}
	i := 0
	for _, f := range ft.Params.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		if errset.IsError(info.TypeOf(f.Type)) {
			return i
		}
		i += n
	}
	return -1
}

// funcLabel is the display name of a declaration: Name, or Recv.Name.
func funcLabel(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	t := fd.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if id, ok := t.(*ast.IndexExpr); ok {
		t = id.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

// mapperInfo builds the -mappers entry of a mapper.
func (a *analyzer) mapperInfo(kind string, fd *ast.FuncDecl, fn *types.Func, bs []mapBranch, def int, hasDef bool) MapperInfo {
	m := MapperInfo{Kind: kind, Name: funcLabel(fd) + " (" + a.posStr(fd.Name.Pos()) + ")", Default: def, HasDefault: hasDef}
	for _, b := range bs {
		desc := b.Br.Key
		if desc == "" {
			desc = b.Br.Match
		}
		m.Branches = append(m.Branches, desc+" -> "+statusText(b.Status))
	}
	return m
}

func statusText(s int) string {
	if s == 0 {
		return "unknown"
	}
	return strconv.Itoa(s)
}

// dedupe drops repeated findings (the same site, variant and status).
func dedupe(in []Finding) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, f := range in {
		k := f.Kind + "|" + f.Pos + "|" + f.Variant + "|" + f.Key + "|" + strconv.Itoa(f.Status) + "|" + f.Handler
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

// branchObs is the observation of a mapper branch: its variant, its status and
// the position of the return or sink that gives it.
func branchObs(b mapBranch) Obs {
	return Obs{Key: b.Br.Key, Status: b.Status, Pos: b.Pos}
}

// dedupeObs drops repeated observations (the same key, status and site). A
// mapper's branches are read both as its own declaration and as a handler.
func dedupeObs(in []Obs) []Obs {
	seen := map[Obs]bool{}
	var out []Obs
	for _, o := range in {
		if !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	return out
}

// isNil reports whether e is the predeclared nil.
func isNil(info *types.Info, e ast.Expr) bool {
	tv, ok := info.Types[ast.Unparen(e)]
	return ok && tv.IsNil()
}

// ident returns the object of an identifier expression, or nil.
func ident(info *types.Info, e ast.Expr) (*ast.Ident, types.Object) {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok {
		return nil, nil
	}
	return id, info.Uses[id]
}

// keyPkgOf splits the package path of a variant key.
func keyPkgOf(key string) string {
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		return ""
	}
	return key[:i]
}

// inModule reports whether a package path belongs to the module.
func inModule(pkg, module string) bool {
	return module != "" && (pkg == module || strings.HasPrefix(pkg, module+"/"))
}
