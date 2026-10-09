// Package errset is a go/analysis analyzer that infers the set of error
// variants a Go function can return, lets developers declare error
// contracts, and checks that every error an HTTP handler can produce is
// covered by the project's error-to-HTTP mapper.
//
// Directives:
//
//	//effectgo:errors A, B, pkg.C, ?, iface:pkg.I
//	        on a function: every inferred variant must be allowed.
//	        on an interface method, or on an interface type (all methods;
//	        a method-level directive overrides it): the method's contract.
//	//effectgo:errmap                   the function is func(error) error mapping errors to HTTP
//	//effectgo:handler M                 the function is an HTTP handler checked against errmap M
//
// Variants are sentinels (package-level error vars), error types (named
// non-interface types implementing error), abstract interface variants
// ("iface:pkg.I": some error whose type implements I), "param:i" (an error
// produced by calling the function's own func parameter i, substituted at
// call sites), "new@pkg.F" (a fresh error created in F without wrapping: an
// errors.New or a constant-format fmt.Errorf without %w, which no errors.Is
// or errors.As can match) and "?" (opaque: origin unknown). A variant whose
// identifier is unexported and declared outside the analyzed module is shown
// as "?" (see collapseForeign).
//
// Stub files (see stubs.go) can state the error set of a function or
// method; they take precedence over contracts and inference.
package errset

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/faintghost/effectgo/internal/goroot"
	"github.com/faintghost/effectgo/internal/stubs"
)

// Kinds of variant.
const (
	KindSentinel = "sentinel"
	KindType     = "type"
	KindIface    = "iface"
	KindParam    = "param"
	KindOpaque   = "opaque"
	KindFresh    = "fresh"
)

const (
	opaqueKey   = "?"
	ifacePrefix = "iface:"
	paramPrefix = "param:"
	freshPrefix = "new@"
	// freshContract is the //effectgo:errors name that allows fresh variants.
	freshContract = "new"
)

// Variant is one error that a function may return.
type Variant struct {
	Key     string   // "pkgpath.Name" for sentinels and types; "iface:pkgpath.I"; "param:i"; "new:pkgpath.F" for fresh; "?" for opaque
	Name    string   // display name: "domain.ErrNotFound", "domain.ValidationError", "some fuego.ErrorWithStatus", "new@domain.F", "param:1", "?"
	Kind    string   // one of the Kind constants
	Methods []string // method set as "Name(params)(results)"; used to match interface As targets
	Chain   []string // witness: callee names from the reporting function down to the origin function
	// Status is the HTTP status that Fuego gives this error when no errmap maps
	// it, from the StatusCode method of its type (or a stub). 0 when unknown.
	Status int
	// DynamicStatus is true for a type whose StatusCode is not a constant (its
	// status depends on the value); such a variant has no known status.
	DynamicStatus bool
	// Pkg and Ident identify a fresh variant's creating function: its package
	// path and the function's own (last) name. For the other kinds they are
	// derived from Key (see origin).
	Pkg   string
	Ident string
	// Orig is the original display name of a foreign unexported variant that
	// was collapsed to "?" (see collapseForeign). It is shown in witness lines only.
	Orig string
	// Carried is true for an error that a status rule names as carrying its own
	// status (huma.Error404NotFound): Status is the status the value carries, and
	// a passthrough errmap branch keeps it (see carried.go).
	Carried bool
}

// ErrFact is exported on every function with a body in a analyzed package.
type ErrFact struct {
	Errors     []Variant // error result set (may contain param:i)
	HasHandler bool      // the function returns at least one func literal with an error result
	Handler    []Variant // union of the error sets of those returned literals
}

// AFact marks ErrFact as an analysis fact.
func (*ErrFact) AFact() {}

func (f *ErrFact) String() string {
	return "errset: errors=" + variantNames(f.Errors) + " handler=" + variantNames(f.Handler)
}

// ContractFact is exported on an interface method that has a //effectgo:errors contract.
type ContractFact struct {
	Allowed []Variant
}

// AFact marks ContractFact as an analysis fact.
func (*ContractFact) AFact() {}

func (f *ContractFact) String() string {
	return "errset-contract: " + variantNames(f.Allowed)
}

// Iface is an interface that an errors.As call targets. A variant counts as
// mapped if its method set includes all of Methods.
type Iface struct {
	Key     string // "pkgpath.Name", or "" for an unnamed interface
	Name    string
	Methods []string
}

// ErrmapFact is exported on //effectgo:errmap functions.
type ErrmapFact struct {
	Mapped   []string // variant keys matched by errors.Is, == / case, errors.As on a concrete type
	Ifaces   []Iface  // interface targets of errors.As / type switches
	Branches []Branch // the rules in the order they are tried; they give the HTTP status (see status.go)
}

// AFact marks ErrmapFact as an analysis fact.
func (*ErrmapFact) AFact() {}

func (f *ErrmapFact) String() string {
	var names []string
	for _, i := range f.Ifaces {
		names = append(names, "iface:"+i.Name)
	}
	return "errset-errmap: mapped=" + strings.Join(f.Mapped, ",") + " " + strings.Join(names, ",")
}

// Result is the ResultType of the errset analyzer. The signature and openapi
// analyzers read it. Facts are not shared between analyzers, so the result is
// the only way to read this package's errmaps and literals from another
// analyzer.
type Result struct {
	Funcs    map[*types.Func]FuncErrors  // every function with a body
	Literals map[*ast.FuncLit]FuncErrors // every func literal with an error result, in this package
	Errmaps  map[*types.Func]*ErrmapFact // every valid //effectgo:errmap function of this package
	// Flows is the error flow of every declared function with a body (see Flow).
	// The gin adapter reads the sets of a handler's local error variables here.
	Flows map[*ast.FuncDecl]Flow
	// Mappers holds the branches of every func(error) error of this package,
	// with or without a directive: the evidence that a mapper maps a variant to
	// a status (see MappingsOf). Unlike Errmaps, it is not a claim of coverage.
	Mappers map[*types.Func]*ErrmapFact
}

// FuncErrors is the error set of one function. When the function returns
// handler literals (Handler is true), Variants is the handler set, which is
// what an HTTP boundary check sees; otherwise it is the function's own set.
type FuncErrors struct {
	Variants []Variant
	Handler  bool
	// HandlerDirective is the argument of the //effectgo:handler directive as
	// written ("httpres.ToHTTPError"), or "" when there is none.
	HandlerDirective string
	// Errmap is the errmap function that the directive names, or nil when there
	// is no directive or it does not resolve to a function.
	Errmap *types.Func
}

// Analyzer is the errset analyzer.
var Analyzer = &analysis.Analyzer{
	Name:       "errset",
	Doc:        "infers error sets of Go functions, checks //effectgo:errors contracts and //effectgo:handler coverage by //effectgo:errmap",
	Run:        run,
	FactTypes:  []analysis.Fact{new(ErrFact), new(ErrmapFact), new(ContractFact), new(StatusFact)},
	ResultType: reflect.TypeOf(Result{}),
}

// effectsKinds are the directive keywords of the effects analyzer. Both
// analyzers read the //effectgo: prefix; each skips the other's keywords.
var effectsKinds = map[string]bool{"pure": true, "allow": true, "package": true}

// Flags of the errset analyzer. The stub files are shared (-stubs, see
// internal/stubs), so there is no errset.stubs flag.
var (
	reportFlag       bool
	witnessFlag      bool
	ignoreOpaqueFlag bool
	statusesFlag     bool
	engineFlag       string
)

func init() {
	Analyzer.Flags.BoolVar(&reportFlag, "report", false, "print the inferred error set of every function with an error result")
	Analyzer.Flags.BoolVar(&witnessFlag, "witness", false, "print one \"witness of <F>: <variant> via <chain>\" line per variant of the error sets")
	Analyzer.Flags.BoolVar(&ignoreOpaqueFlag, "ignoreopaque", false, "do not report opaque (\"?\") and fresh (new@) errors in handler checks")
	Analyzer.Flags.BoolVar(&statusesFlag, "statuses", false, "print the HTTP status of every variant of each //effectgo:handler function")
	Analyzer.Flags.StringVar(&engineFlag, "engine", engineAST, "error-set engine: ast (flow-insensitive) or ssa (flow-sensitive, SSA values)")
}

var (
	errorType  = types.Universe.Lookup("error").Type()
	errorIface = errorType.Underlying().(*types.Interface)
)

// ---- errSet ----

// errSet maps a variant key to its variant. Each key keeps the shortest
// witness chain seen so far.
type errSet map[string]Variant

// put adds v and reports whether the set changed.
func (s errSet) put(v Variant) bool {
	old, ok := s[v.Key]
	if !ok || len(v.Chain) < len(old.Chain) {
		s[v.Key] = v
		return true
	}
	return false
}

func (s errSet) putAll(o errSet) bool {
	changed := false
	for _, v := range o {
		if s.put(v) {
			changed = true
		}
	}
	return changed
}

// prefixed returns a copy of s with name prepended to every witness chain.
func (s errSet) prefixed(name string) errSet {
	out := errSet{}
	for _, v := range s {
		v.Chain = append([]string{name}, v.Chain...)
		out.put(v)
	}
	return out
}

func (s errSet) list() []Variant {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Variant, 0, len(keys))
	for _, k := range keys {
		out = append(out, s[k])
	}
	return out
}

func setOf(vs ...Variant) errSet {
	s := errSet{}
	for _, v := range vs {
		s.put(v)
	}
	return s
}

func opaqueVariant(chain ...string) Variant {
	return Variant{Key: opaqueKey, Name: "?", Kind: KindOpaque, Chain: chain}
}

func paramVariant(i int) Variant {
	name := paramPrefix + strconv.Itoa(i)
	return Variant{Key: name, Name: name, Kind: KindParam}
}

// paramIndex returns i for a param:i variant, or -1.
func paramIndex(v Variant) int {
	if v.Kind != KindParam {
		return -1
	}
	i, err := strconv.Atoi(strings.TrimPrefix(v.Key, paramPrefix))
	if err != nil {
		return -1
	}
	return i
}

// displayOrder sorts variants by name, with the opaque "?" last.
func displayOrder(vs []Variant) []Variant {
	out := append([]Variant(nil), vs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Kind == KindOpaque) != (b.Kind == KindOpaque) {
			return b.Kind == KindOpaque
		}
		return a.Name < b.Name
	})
	return out
}

// variantNames renders a set for the report: display names in order, each
// name once (several collapsed foreign variants all show as "?").
func variantNames(vs []Variant) string {
	if len(vs) == 0 {
		return "(none)"
	}
	seen := map[string]bool{}
	var names []string
	for _, v := range displayOrder(vs) {
		if seen[v.Name] {
			continue
		}
		seen[v.Name] = true
		names = append(names, v.Name)
	}
	return strings.Join(names, " | ")
}

func via(chain []string) string {
	if len(chain) == 0 {
		return ""
	}
	return " (via " + joinChain(chain) + ")"
}

// oneLine makes a chain element fit on one line: whitespace runs, including
// the newlines of a multi-line call such as tx.QueryRow(ctx, `SELECT …`).Scan,
// become one space, and an element longer than 80 characters is cut. The
// witness lines are grepped by line, so an element must not break one.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 80 {
		s = string(r[:77]) + "..."
	}
	return s
}

// joinChain renders a witness chain with " → " between one-line elements.
func joinChain(chain []string) string {
	parts := make([]string, len(chain))
	for i, c := range chain {
		parts[i] = oneLine(c)
	}
	return strings.Join(parts, " → ")
}

// witnessName is the name of a variant in a witness line. A collapsed foreign
// variant keeps its original name there: "? (pgconn.connLockError)".
func (v Variant) witnessName() string {
	if v.Orig != "" {
		return "? (" + v.Orig + ")"
	}
	return v.Name
}

// chainText renders a witness chain for a witness line; the reporting
// function itself is not part of the chain.
func chainText(chain []string) string {
	if len(chain) == 0 {
		return "(direct)"
	}
	return joinChain(chain)
}

// origin returns the package path and the identifier of the object that a
// variant names. Opaque and param variants have none (ok is false).
func origin(v Variant) (pkg, ident string, ok bool) {
	switch v.Kind {
	case KindFresh:
		return v.Pkg, v.Ident, v.Pkg != ""
	case KindSentinel, KindType, KindIface:
		key := strings.TrimPrefix(v.Key, ifacePrefix)
		i := strings.LastIndex(key, ".")
		if i < 0 {
			return "", "", false
		}
		return key[:i], key[i+1:], true
	}
	return "", "", false
}

// collapseForeign returns the variants as user code can see them. A sentinel,
// type or interface variant whose identifier is unexported and whose package
// lies outside module can never be named, matched or asserted by the analyzed
// code, so it is shown as the opaque "?". A fresh variant created by a function
// of a foreign package is collapsed whatever the creator's name: a fresh error
// is never matchable, so the creator names only dependency internals (for
// example new@pgconn.Pipeline.Flush). The original name is kept in Orig for
// witness lines. The status of a known type is kept. An empty module disables
// the collapse. Exported foreign sentinels and types (pgx.ErrNoRows,
// context.Canceled, *url.Error) are kept.
func collapseForeign(vs []Variant, module string) []Variant {
	if module == "" {
		return vs
	}
	out := make([]Variant, 0, len(vs))
	for _, v := range vs {
		pkg, ident, ok := origin(v)
		foreign := ok && !inModule(pkg, module)
		if !foreign || (v.Kind != KindFresh && isExported(ident)) {
			out = append(out, v)
			continue
		}
		out = append(out, Variant{
			Key: opaqueKey + " " + v.Key, Name: "?", Kind: KindOpaque, Chain: v.Chain,
			Status: v.Status, DynamicStatus: v.DynamicStatus, Orig: v.Name,
		})
	}
	return out
}

// isExported reports whether the identifier is exported.
func isExported(ident string) bool {
	r, _ := utf8.DecodeRuneInString(ident)
	return unicode.IsUpper(r)
}

// inModule reports whether the package path belongs to the module path.
func inModule(pkg, module string) bool {
	return pkg == module || strings.HasPrefix(pkg, module+"/")
}

// ---- type helpers ----

func implementsError(t types.Type) bool {
	if t == nil {
		return false
	}
	return types.Implements(t, errorIface) || types.Implements(types.NewPointer(t), errorIface)
}

// namedErrorType returns the named non-interface type T for T or *T when T implements error.
func namedErrorType(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	n, ok := t.(*types.Named)
	if !ok || types.IsInterface(n) || !implementsError(n) {
		return nil
	}
	return n
}

func objKey(o types.Object) string {
	if o.Pkg() == nil {
		return o.Name()
	}
	return o.Pkg().Path() + "." + o.Name()
}

func objDisplay(o types.Object) string {
	if o.Pkg() == nil {
		return o.Name()
	}
	return o.Pkg().Name() + "." + o.Name()
}

// methodsOf returns the method set of t (pointer method set for concrete
// types, the declared methods for interfaces).
func methodsOf(t types.Type) []string {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	var out []string
	if types.IsInterface(t) {
		iface := t.Underlying().(*types.Interface)
		for i := 0; i < iface.NumMethods(); i++ {
			m := iface.Method(i)
			out = append(out, sigKey(m.Name(), m.Type().(*types.Signature)))
		}
	} else {
		ms := types.NewMethodSet(types.NewPointer(t))
		for i := 0; i < ms.Len(); i++ {
			f := ms.At(i).Obj().(*types.Func)
			out = append(out, sigKey(f.Name(), f.Type().(*types.Signature)))
		}
	}
	sort.Strings(out)
	return out
}

func sigKey(name string, sig *types.Signature) string {
	q := func(p *types.Package) string { return p.Path() }
	var b strings.Builder
	b.WriteString(name)
	b.WriteString("(")
	for i := 0; i < sig.Params().Len(); i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(types.TypeString(sig.Params().At(i).Type(), q))
	}
	b.WriteString(")(")
	for i := 0; i < sig.Results().Len(); i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(types.TypeString(sig.Results().At(i).Type(), q))
	}
	b.WriteString(")")
	return b.String()
}

// typeVariant is the variant of a named error type, with its HTTP status.
func (p *pkgState) typeVariant(n *types.Named) Variant {
	v := Variant{Key: objKey(n.Obj()), Name: objDisplay(n.Obj()), Kind: KindType, Methods: methodsOf(n)}
	v.Status, v.DynamicStatus = p.typeStatus(n)
	return v
}

// ifaceVariant is the abstract variant "some pkg.I" for a named interface.
func ifaceVariant(n *types.Named) Variant {
	return Variant{Key: ifacePrefix + objKey(n.Obj()), Name: "some " + objDisplay(n.Obj()), Kind: KindIface, Methods: methodsOf(n)}
}

// variantFromObject turns a package-level object into a variant of the given
// kind. It reports false when the object does not have that kind.
func (p *pkgState) variantFromObject(obj types.Object, kind string) (Variant, bool) {
	switch kind {
	case KindSentinel:
		v, ok := sentinelKey(obj)
		if !ok {
			return Variant{}, false
		}
		return Variant{Key: objKey(v), Name: objDisplay(v), Kind: KindSentinel, Methods: methodsOf(v.Type())}, true
	case KindType:
		tn, ok := obj.(*types.TypeName)
		if !ok {
			return Variant{}, false
		}
		n := namedErrorType(tn.Type())
		if n == nil {
			return Variant{}, false
		}
		return p.typeVariant(n), true
	case KindIface:
		tn, ok := obj.(*types.TypeName)
		if !ok {
			return Variant{}, false
		}
		n, ok := types.Unalias(tn.Type()).(*types.Named)
		if !ok || !types.IsInterface(n) {
			return Variant{}, false
		}
		return ifaceVariant(n), true
	}
	return Variant{}, false
}

func sentinelKey(obj types.Object) (*types.Var, bool) {
	v, ok := obj.(*types.Var)
	if !ok || v.Pkg() == nil || v.IsField() || v.Parent() != v.Pkg().Scope() {
		return nil, false
	}
	if !implementsError(v.Type()) {
		return nil, false
	}
	return v, true
}

func funcName(f *types.Func) string {
	if recv := f.Type().(*types.Signature).Recv(); recv != nil {
		t := types.Unalias(recv.Type())
		if p, ok := t.(*types.Pointer); ok {
			t = types.Unalias(p.Elem())
		}
		if n, ok := t.(*types.Named); ok {
			return n.Obj().Name() + "." + f.Name()
		}
	}
	return f.Name()
}

// recvTypeName returns the name of the named receiver type of a method, or "".
func recvTypeName(f *types.Func) string {
	recv := f.Type().(*types.Signature).Recv()
	if recv == nil {
		return ""
	}
	t := types.Unalias(recv.Type())
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}

func isInterfaceMethod(f *types.Func) bool {
	recv := f.Type().(*types.Signature).Recv()
	return recv != nil && types.IsInterface(recv.Type())
}

// stdCache maps a *types.Package to whether it is standard library.
var stdCache sync.Map

// isStd reports whether pkg is a standard library package, by the source
// directory its objects were declared in (see internal/goroot). Facts exist for
// std packages too, so std callees must be checked explicitly.
func isStd(fset *token.FileSet, pkg *types.Package) bool {
	if v, ok := stdCache.Load(pkg); ok {
		return v.(bool)
	}
	std := !strings.Contains(pkg.Path(), ".") && goroot.PkgIsStd(fset, pkg)
	stdCache.Store(pkg, std)
	return std
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// errIndex returns the index of the last result implementing error, or -1.
func errIndex(sig *types.Signature) int {
	for i := sig.Results().Len() - 1; i >= 0; i-- {
		if implementsError(sig.Results().At(i).Type()) {
			return i
		}
	}
	return -1
}

// walkOwn visits body without descending into nested function literals.
func walkOwn(body ast.Node, f func(ast.Node)) {
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok && n != body {
			return false
		}
		f(n)
		return true
	})
}

func paramVars(info *types.Info, recv *ast.FieldList, ft *ast.FuncType, set map[*types.Var]bool) {
	for _, fl := range []*ast.FieldList{recv, ft.Params} {
		if fl == nil {
			continue
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				if v, ok := info.Defs[n].(*types.Var); ok {
					set[v] = true
				}
			}
		}
	}
}

// ownParamIndex maps each parameter of a declared function (not of nested
// literals) to its 0-based position in the parameter list. The receiver is
// not counted.
func ownParamIndex(info *types.Info, ft *ast.FuncType) map[*types.Var]int {
	out := map[*types.Var]int{}
	if ft.Params == nil {
		return out
	}
	idx := 0
	for _, f := range ft.Params.List {
		if len(f.Names) == 0 {
			idx++
			continue
		}
		for _, n := range f.Names {
			if v, ok := info.Defs[n].(*types.Var); ok {
				out[v] = idx
			}
			idx++
		}
	}
	return out
}

// ---- per-unit flow analysis ----

type flowRec struct {
	v      *types.Var
	src    ast.Expr   // assigned expression
	typ    types.Type // type-switch case type, if src is nil
	opaque bool
}

type flowCtx struct {
	p         *pkgState
	info      *types.Info
	owner     string // name of the declared function being analyzed; "" if none (fresh errors need it)
	recs      []flowRec
	flow      map[*types.Var]errSet // error set of each local error variable, flow-insensitive
	params    map[*types.Var]bool   // every parameter of the unit and of nested literals
	ownParams map[*types.Var]int    // parameters of the declared function only
}

func (p *pkgState) newFlow(u *fnUnit) *flowCtx {
	recv, ft, body := u.decl.Recv, u.decl.Type, u.decl.Body
	c := &flowCtx{p: p, info: p.pass.TypesInfo, owner: funcName(u.obj), flow: map[*types.Var]errSet{}, params: map[*types.Var]bool{}}
	paramVars(c.info, recv, ft, c.params)
	c.ownParams = ownParamIndex(c.info, ft)
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			paramVars(c.info, nil, x.Type, c.params)
		case *ast.AssignStmt:
			c.assign(x.Lhs, x.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, len(x.Names))
			for i, id := range x.Names {
				lhs[i] = id
			}
			if len(x.Values) > 0 {
				c.assign(lhs, x.Values)
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{x.Key, x.Value} {
				if v := c.localVar(e); v != nil && implementsError(v.Type()) {
					c.recs = append(c.recs, flowRec{v: v, opaque: true})
				}
			}
		case *ast.TypeSwitchStmt:
			for _, s := range x.Body.List {
				cc := s.(*ast.CaseClause)
				v, ok := c.info.Implicits[cc].(*types.Var)
				if !ok {
					continue
				}
				if len(cc.List) == 1 {
					c.recs = append(c.recs, flowRec{v: v, typ: c.info.TypeOf(cc.List[0])})
				} else {
					c.recs = append(c.recs, flowRec{v: v, opaque: true})
				}
			}
		}
		return true
	})
	c.solve()
	return c
}

func (c *flowCtx) localVar(e ast.Expr) *types.Var {
	id, ok := unparen(e).(*ast.Ident)
	if !ok {
		return nil
	}
	v, ok := c.info.ObjectOf(id).(*types.Var)
	if !ok || v.Pkg() == nil || v.Parent() == v.Pkg().Scope() {
		return nil
	}
	return v
}

func (c *flowCtx) assign(lhs, rhs []ast.Expr) {
	for i, l := range lhs {
		v := c.localVar(l)
		if v == nil || !implementsError(v.Type()) {
			continue
		}
		switch {
		case len(lhs) == len(rhs):
			c.recs = append(c.recs, flowRec{v: v, src: rhs[i]})
		case len(rhs) == 1:
			c.recs = append(c.recs, flowRec{v: v, src: rhs[0]})
		default:
			c.recs = append(c.recs, flowRec{v: v, opaque: true})
		}
	}
}

func (c *flowCtx) solve() {
	for changed := true; changed; {
		changed = false
		for _, r := range c.recs {
			var add errSet
			switch {
			case r.opaque:
				add = setOf(opaqueVariant())
			case r.typ != nil:
				add = c.typeSet(r.typ)
			default:
				add = c.exprSet(r.src)
			}
			dst := c.flow[r.v]
			if dst == nil {
				dst = errSet{}
				c.flow[r.v] = dst
			}
			if dst.putAll(add) {
				changed = true
			}
		}
	}
}

func (c *flowCtx) typeSet(t types.Type) errSet {
	if n := namedErrorType(t); n != nil {
		return setOf(c.p.typeVariant(n))
	}
	return setOf(opaqueVariant())
}

// exprSet returns the error set of an expression of error type.
func (c *flowCtx) exprSet(e ast.Expr) errSet {
	e = unparen(e)
	if tv, ok := c.info.Types[e]; ok && tv.IsNil() {
		return errSet{}
	}
	if t := c.compositeType(e); t != nil {
		return c.typeSet(t)
	}
	switch x := e.(type) {
	case *ast.Ident:
		return c.identSet(c.info.Uses[x])
	case *ast.SelectorExpr:
		if v, ok := c.info.Uses[x.Sel].(*types.Var); ok && v.IsField() {
			return setOf(opaqueVariant())
		}
		return c.identSet(c.info.Uses[x.Sel])
	case *ast.CallExpr:
		return c.callSet(x)
	}
	return setOf(opaqueVariant())
}

// compositeType returns the type of T{}, &T{} or new(T) so the caller can
// treat it as a variant. It returns nil for every other expression.
func (c *flowCtx) compositeType(e ast.Expr) types.Type {
	switch x := e.(type) {
	case *ast.CompositeLit:
		return c.info.TypeOf(x)
	case *ast.UnaryExpr:
		if _, ok := unparen(x.X).(*ast.CompositeLit); ok && x.Op == token.AND {
			return c.info.TypeOf(x)
		}
	case *ast.CallExpr:
		if id, ok := unparen(x.Fun).(*ast.Ident); ok {
			if b, ok := c.info.Uses[id].(*types.Builtin); ok && b.Name() == "new" {
				return c.info.TypeOf(x)
			}
		}
	}
	return nil
}

func (c *flowCtx) identSet(obj types.Object) errSet {
	v, ok := obj.(*types.Var)
	if !ok {
		return setOf(opaqueVariant())
	}
	if sv, ok := sentinelKey(v); ok {
		return setOf(Variant{Key: objKey(sv), Name: objDisplay(sv), Kind: KindSentinel, Methods: methodsOf(sv.Type())})
	}
	if v.Pkg() != nil && v.Parent() == v.Pkg().Scope() {
		return setOf(opaqueVariant()) // package var that is not an error sentinel
	}
	if c.params[v] {
		return setOf(opaqueVariant())
	}
	out := errSet{}
	out.putAll(c.flow[v])
	return out
}

func (c *flowCtx) callSet(call *ast.CallExpr) errSet {
	if tv, ok := c.info.Types[call.Fun]; ok && tv.IsType() {
		return setOf(opaqueVariant()) // conversion
	}
	if lit, ok := unparen(call.Fun).(*ast.FuncLit); ok {
		// func() error {...}(): the literal runs here, so its own returns
		// are the call's error set.
		if sig, ok := c.info.TypeOf(lit).(*types.Signature); ok && errIndex(sig) >= 0 {
			return c.returnsErr(sig, lit.Body)
		}
		return errSet{}
	}
	callee := typeutil.StaticCallee(c.info, call)
	if callee == nil {
		return c.dynamicCallSet(call)
	}
	callee = callee.Origin()
	if v, ok := c.p.carriedOfCall(callee, call); ok {
		return setOf(v)
	}
	if s, ok := c.p.stubSet(callee); ok {
		return s.prefixed(stubLabel(callee))
	}
	name := funcName(callee)
	pkg := callee.Pkg()
	if pkg != nil {
		switch {
		case pkg.Path() == "fmt" && callee.Name() == "Errorf":
			return c.errorfSet(call)
		case pkg.Path() == "errors" && callee.Name() == "Join":
			return c.joinSet(call)
		case pkg.Path() == "errors" && callee.Name() == "New":
			return c.freshSet("errors.New")
		}
	}
	if pkg == nil {
		return setOf(opaqueVariant(name))
	}
	if isStd(c.p.pass.Fset, pkg) {
		return setOf(opaqueVariant(pkg.Name() + "." + name))
	}
	if pkg == c.p.pass.Pkg {
		if w, ok := c.p.work[callee]; ok {
			return c.bind(w.errs, call, name)
		}
		return setOf(opaqueVariant(name))
	}
	var f ErrFact
	if c.p.pass.ImportObjectFact(callee, &f) {
		return c.bind(setOf(f.Errors...), call, name)
	}
	return setOf(opaqueVariant(pkg.Name() + "." + name))
}

// dynamicCallSet handles calls that have no static callee: calls of the
// declared function's own parameters, calls of interface methods (contract
// or opaque) and calls of other func values (opaque).
func (c *flowCtx) dynamicCallSet(call *ast.CallExpr) errSet {
	fun := unparen(call.Fun)
	if v, ok := c.p.carriedOfVarCall(call); ok {
		return setOf(v)
	}
	if s, label, ok := c.p.dynamicStubSet(c.info, call); ok {
		return s.prefixed(label)
	}
	if id, ok := fun.(*ast.Ident); ok {
		if v, ok := c.info.Uses[id].(*types.Var); ok {
			if i, ok := c.ownParams[v]; ok {
				return setOf(paramVariant(i))
			}
		}
	}
	if sel, ok := fun.(*ast.SelectorExpr); ok {
		if m, ok := c.info.Uses[sel.Sel].(*types.Func); ok && isInterfaceMethod(m) {
			m = m.Origin()
			if s, ok := c.p.stubSet(m); ok {
				return s.prefixed(stubLabel(m))
			}
			if allowed, ok := c.p.contractOf(m); ok {
				label := "(" + recvTypeName(m) + ")." + m.Name() + " [contract]"
				out := errSet{}
				for _, v := range allowed {
					v.Chain = []string{label}
					out.put(v)
				}
				return out
			}
		}
	}
	return setOf(opaqueVariant(types.ExprString(call.Fun)))
}

// bind substitutes the param:i variants of a callee's error set with the
// error sets of the call's arguments, and prefixes every witness with the
// callee name. An argument that cannot be followed gives "?".
func (c *flowCtx) bind(s errSet, call *ast.CallExpr, name string) errSet {
	out := errSet{}
	for _, v := range s {
		if v.Kind != KindParam {
			v.Chain = append([]string{name}, v.Chain...)
			out.put(v)
			continue
		}
		i := paramIndex(v)
		if i < 0 || i >= len(call.Args) || call.Ellipsis.IsValid() {
			out.put(opaqueVariant(name))
			continue
		}
		out.putAll(c.argSet(call.Args[i]).prefixed(name))
	}
	return out
}

// argSet is the error set that a func-typed argument can produce when called.
func (c *flowCtx) argSet(arg ast.Expr) errSet {
	switch x := unparen(arg).(type) {
	case *ast.FuncLit:
		sig, ok := c.info.TypeOf(x).(*types.Signature)
		if !ok || errIndex(sig) < 0 {
			return errSet{}
		}
		return c.returnsErr(sig, x.Body)
	case *ast.Ident, *ast.SelectorExpr:
		var obj types.Object
		if id, ok := x.(*ast.Ident); ok {
			obj = c.info.Uses[id]
		} else {
			obj = c.info.Uses[x.(*ast.SelectorExpr).Sel]
		}
		if v, ok := obj.(*types.Var); ok {
			if i, ok := c.ownParams[v]; ok {
				return setOf(paramVariant(i)) // the caller's own parameter: composes
			}
		}
		if fn, ok := obj.(*types.Func); ok {
			return c.namedFuncSet(fn.Origin())
		}
	}
	return setOf(opaqueVariant())
}

// namedFuncSet is the error set of a named function or method used as a value.
// Its own unresolved parameters cannot be bound here, so they become "?".
func (c *flowCtx) namedFuncSet(fn *types.Func) errSet {
	return c.p.namedFuncSet(fn)
}

// namedFuncSet is the error set of a named function used as a value (see
// flowCtx.namedFuncSet). It is shared by both engines.
func (p *pkgState) namedFuncSet(fn *types.Func) errSet {
	if s, ok := p.stubSet(fn); ok {
		return s.prefixed(stubLabel(fn))
	}
	label := funcName(fn)
	var src []Variant
	switch {
	case fn.Pkg() == nil || isStd(p.pass.Fset, fn.Pkg()):
		return setOf(opaqueVariant(label))
	case fn.Pkg() == p.pass.Pkg:
		w, ok := p.work[fn]
		if !ok {
			return setOf(opaqueVariant(label))
		}
		src = w.errs.list()
	default:
		var f ErrFact
		if !p.pass.ImportObjectFact(fn, &f) {
			return setOf(opaqueVariant(label))
		}
		src = f.Errors
	}
	out := errSet{}
	for _, v := range src {
		if v.Kind == KindParam {
			out.put(opaqueVariant(label))
			continue
		}
		v.Chain = append([]string{label}, v.Chain...)
		out.put(v)
	}
	return out
}

// freshSet is the error set of an error created in place by the declared
// function being analyzed (errors.New, or fmt.Errorf with a constant format and
// no %w). It is one variant per creating function: "new@pkg.F". Outside a
// declared function (no owner) it is opaque, as before.
func (c *flowCtx) freshSet(origin string) errSet {
	if c.owner == "" {
		return setOf(opaqueVariant(origin))
	}
	return setOf(c.p.freshVariant(c.owner))
}

// lastComponent returns the part of a function display name after the last dot.
func lastComponent(name string) string {
	return name[strings.LastIndex(name, ".")+1:]
}

// errorfSet: fmt.Errorf is the union of its %w arguments. A constant format
// with no %w verb creates a fresh error. Without a constant format (or one
// with explicit argument indexes) the result is opaque.
func (c *flowCtx) errorfSet(call *ast.CallExpr) errSet {
	fail := setOf(opaqueVariant("fmt.Errorf"))
	if len(call.Args) == 0 {
		return fail
	}
	tv := c.info.Types[call.Args[0]]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		return fail
	}
	idxs, ok := wrapVerbArgs(constant.StringVal(tv.Value))
	if !ok {
		return fail
	}
	if len(idxs) == 0 {
		return c.freshSet("fmt.Errorf")
	}
	if call.Ellipsis.IsValid() {
		return fail
	}
	out := errSet{}
	for _, i := range idxs {
		if i+1 >= len(call.Args) {
			return fail
		}
		out.putAll(c.exprSet(call.Args[i+1]))
	}
	return out
}

// wrapVerbArgs returns the 0-based variadic argument indexes consumed by %w
// verbs. It reports false for formats it cannot follow (explicit indexes).
func wrapVerbArgs(s string) ([]int, bool) {
	var idxs []int
	arg := 0
	isDigit := func(b byte) bool { return b >= '0' && b <= '9' }
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		i++
		if i >= len(s) {
			return nil, false
		}
		if s[i] == '%' {
			continue
		}
		for i < len(s) && strings.IndexByte("+-# 0", s[i]) >= 0 {
			i++
		}
		if i < len(s) && s[i] == '*' {
			arg++
			i++
		} else {
			for i < len(s) && isDigit(s[i]) {
				i++
			}
		}
		if i < len(s) && s[i] == '.' {
			i++
			if i < len(s) && s[i] == '*' {
				arg++
				i++
			} else {
				for i < len(s) && isDigit(s[i]) {
					i++
				}
			}
		}
		if i >= len(s) || s[i] == '[' {
			return nil, false
		}
		if s[i] == 'w' {
			idxs = append(idxs, arg)
		}
		arg++
	}
	return idxs, true
}

// joinSet: errors.Join is the union of its arguments.
func (c *flowCtx) joinSet(call *ast.CallExpr) errSet {
	if call.Ellipsis.IsValid() {
		return setOf(opaqueVariant("errors.Join"))
	}
	out := errSet{}
	for _, a := range call.Args {
		out.putAll(c.exprSet(a))
	}
	return out
}

// returnsErr is the union of the error results of the return statements in
// body (not descending into nested literals).
func (c *flowCtx) returnsErr(sig *types.Signature, body *ast.BlockStmt) errSet {
	out := errSet{}
	idx := errIndex(sig)
	if idx < 0 {
		return out
	}
	var named *types.Var
	if rv := sig.Results().At(idx); rv.Name() != "" && rv.Name() != "_" {
		named = rv
	}
	walkOwn(body, func(n ast.Node) {
		r, ok := n.(*ast.ReturnStmt)
		if !ok {
			return
		}
		switch {
		case len(r.Results) == 0:
			if named != nil {
				out.putAll(c.identSet(named))
			}
		case len(r.Results) == 1 && sig.Results().Len() > 1:
			out.putAll(c.exprSet(r.Results[0]))
		case idx < len(r.Results):
			out.putAll(c.exprSet(r.Results[idx]))
		}
	})
	return out
}

// ---- package-level analysis ----

type fnUnit struct {
	decl *ast.FuncDecl
	obj  *types.Func
	sig  *types.Signature
}

type work struct {
	errs       errSet
	handler    errSet
	hasHandler bool
}

// effective is the handler set when the function has one, otherwise its error set.
func (w *work) effective() errSet {
	if w.hasHandler {
		return w.handler
	}
	return w.errs
}

type pkgState struct {
	pass      *analysis.Pass
	units     []*fnUnit
	work      map[*types.Func]*work
	contracts map[*types.Func][]Variant // interface method contracts declared in this package
	stubs     map[string]errSet         // stub rules by target key
	pkgIndex  map[string]*types.Package // transitive import graph by path
	// statusDecls holds the constant StatusCode methods of this package.
	statusDecls map[*types.Func]int
	// statusStubs holds the "status" stub rules by canonical type key.
	statusStubs map[string]int
	// errmaps holds the errmap facts of this package, by function.
	errmaps map[*types.Func]*ErrmapFact
	// statusFuncs holds the status rules that name a function or variable, by
	// pkgpath.Name (see carried.go).
	statusFuncs map[string]stubs.StatusValue
	// The SSA engine (see ssa.go): useSSA is set when the package was lowered,
	// ssaProg holds it, callAt maps a call's opening parenthesis to its AST
	// call, and litSets holds the literal sets that the SSA engine computed.
	useSSA  bool
	ssaProg *ssa.Program
	callAt  map[token.Pos]*ast.CallExpr
	litSets map[*ast.FuncLit]FuncErrors
}

func run(pass *analysis.Pass) (any, error) {
	p := &pkgState{pass: pass, work: map[*types.Func]*work{}, contracts: map[*types.Func][]Variant{}, statusDecls: map[*types.Func]int{}, errmaps: map[*types.Func]*ErrmapFact{}, litSets: map[*ast.FuncLit]FuncErrors{}}
	switch engineFlag {
	case engineAST:
	case engineSSA:
		p.useSSA = p.buildSSA()
	default:
		return nil, fmt.Errorf("errset: unknown -errset.engine %q (want ast or ssa)", engineFlag)
	}
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			obj, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func)
			if !ok {
				continue
			}
			p.units = append(p.units, &fnUnit{decl: fd, obj: obj, sig: obj.Type().(*types.Signature)})
		}
	}

	p.collectStatus()
	if err := p.loadStubs(); err != nil {
		return nil, err
	}
	p.collectInterfaceContracts()
	p.solve()
	for _, u := range p.units {
		w := p.work[u.obj]
		pass.ExportObjectFact(u.obj, &ErrFact{
			Errors:     w.errs.list(),
			HasHandler: w.hasHandler,
			Handler:    w.handler.list(),
		})
	}

	for _, u := range p.units {
		for _, d := range directivesOf(u.decl.Doc) {
			if d.kind == "errmap" {
				if f := p.exportErrmap(u); f != nil {
					p.errmaps[u.obj] = f
				}
			}
		}
	}

	for _, u := range p.units {
		p.checkDirectives(u)
	}
	p.checkConformance()

	if statusesFlag {
		for _, u := range p.units {
			if d, ok := handlerOf(u); ok {
				p.reportStatuses(u, d)
			}
		}
	}

	if reportFlag {
		for _, u := range p.units {
			w := p.work[u.obj]
			if errIndex(u.sig) >= 0 {
				pass.Reportf(u.decl.Name.Pos(), "errors of %s: %s", funcName(u.obj), variantNames(p.visible(w.errs.list())))
			}
			if w.hasHandler {
				pass.Reportf(u.decl.Name.Pos(), "handler errors of %s: %s", funcName(u.obj), variantNames(p.visible(w.handler.list())))
			}
		}
	}

	if witnessFlag {
		for _, u := range p.units {
			w := p.work[u.obj]
			if errIndex(u.sig) >= 0 {
				p.reportWitnesses(u, funcName(u.obj), w.errs)
			}
			if w.hasHandler {
				p.reportWitnesses(u, funcName(u.obj)+" [handler]", w.handler)
			}
		}
	}

	flows := map[*ast.FuncDecl]Flow{}
	for _, u := range p.units {
		flows[u.decl] = Flow{p: p, c: p.newFlow(u)}
	}
	res := Result{Funcs: map[*types.Func]FuncErrors{}, Literals: p.literalSets(), Errmaps: p.errmaps, Flows: flows, Mappers: p.mappers()}
	for _, u := range p.units {
		w := p.work[u.obj]
		arg, errmap := p.handlerRef(u)
		res.Funcs[u.obj] = FuncErrors{Variants: p.visible(w.effective().list()), Handler: w.hasHandler, HandlerDirective: arg, Errmap: errmap}
	}
	return res, nil
}

// solve computes the same-package error sets as a fixed point over the call
// graph. Functions with a stub keep the stub's set.
func (p *pkgState) solve() {
	for _, u := range p.units {
		w := &work{errs: errSet{}, handler: errSet{}}
		if s, ok := p.stubSet(u.obj); ok {
			w.errs.putAll(s)
		}
		p.work[u.obj] = w
	}
	for changed := true; changed; {
		changed = false
		for _, u := range p.units {
			if _, ok := p.stubSet(u.obj); ok {
				continue
			}
			errs, handler, has := p.compute(u)
			w := p.work[u.obj]
			if w.errs.putAll(errs) {
				changed = true
			}
			if w.handler.putAll(handler) {
				changed = true
			}
			if has && !w.hasHandler {
				w.hasHandler = true
				changed = true
			}
		}
	}
}

// compute evaluates one function against the current sets of its callees.
func (p *pkgState) compute(u *fnUnit) (errs, handler errSet, hasHandler bool) {
	if p.useSSA {
		if e, h, has, ok := p.computeSSA(u); ok {
			return e, h, has
		}
	}
	c := p.newFlow(u)
	errs = c.returnsErr(u.sig, u.decl.Body)
	handler = errSet{}
	walkOwn(u.decl.Body, func(n ast.Node) {
		r, ok := n.(*ast.ReturnStmt)
		if !ok {
			return
		}
		for _, res := range r.Results {
			lit, ok := unparen(res).(*ast.FuncLit)
			if !ok {
				continue
			}
			ls, ok := c.info.TypeOf(lit).(*types.Signature)
			if !ok || errIndex(ls) < 0 {
				continue
			}
			hasHandler = true
			handler.putAll(c.returnsErr(ls, lit.Body))
		}
	})
	return errs, handler, hasHandler
}

// reportWitnesses prints one witness line per variant of a set, for the
// -witness flag: "witness of <label>: <variant> via <chain>". The chain does
// not include the function itself; "(direct)" means the variant is created in
// the function.
func (p *pkgState) reportWitnesses(u *fnUnit, label string, set errSet) {
	for _, v := range displayOrder(p.visible(set.list())) {
		p.pass.Reportf(u.decl.Name.Pos(), "witness of %s: %s via %s", label, v.witnessName(), chainText(v.Chain))
	}
}

// ---- directives ----

type directive struct {
	kind string
	arg  string
	pos  token.Pos
}

func directivesOf(cg *ast.CommentGroup) []directive {
	if cg == nil {
		return nil
	}
	var out []directive
	for _, c := range cg.List {
		if !strings.HasPrefix(c.Text, "//effectgo:") {
			continue
		}
		body := strings.TrimPrefix(c.Text, "//effectgo:")
		kind, arg, _ := strings.Cut(body, " ")
		if i := strings.Index(arg, "//"); i >= 0 {
			arg = arg[:i]
		}
		out = append(out, directive{kind: strings.TrimSpace(kind), arg: strings.TrimSpace(arg), pos: c.Pos()})
	}
	return out
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (p *pkgState) fileOf(pos token.Pos) *ast.File {
	for _, f := range p.pass.Files {
		if f.Pos() <= pos && pos <= f.End() {
			return f
		}
	}
	return nil
}

// resolve finds the object named by name ("X", "*X", or "pkg.X"). A qualified
// name is resolved through the imports of the file that contains pos.
func (p *pkgState) resolve(name string, pos token.Pos) types.Object {
	name = strings.TrimPrefix(name, "*")
	qual, base, qualified := strings.Cut(name, ".")
	if !qualified {
		return p.pass.Pkg.Scope().Lookup(name)
	}
	file := p.fileOf(pos)
	if file == nil {
		return nil
	}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		for _, ip := range p.pass.Pkg.Imports() {
			if ip.Path() != path {
				continue
			}
			if (alias != "" && alias == qual) || (alias == "" && ip.Name() == qual) {
				return ip.Scope().Lookup(base)
			}
		}
	}
	return nil
}

// contractVariant resolves one name of a //effectgo:errors directive.
func (p *pkgState) contractVariant(name string, pos token.Pos) (Variant, bool) {
	if name == opaqueKey {
		return opaqueVariant(), true
	}
	if name == freshContract {
		return Variant{Key: freshContract, Name: freshContract, Kind: KindFresh}, true
	}
	if rest, ok := strings.CutPrefix(name, ifacePrefix); ok {
		return p.variantFromObject(p.resolve(rest, pos), KindIface)
	}
	obj := p.resolve(name, pos)
	if v, ok := p.variantFromObject(obj, KindSentinel); ok {
		return v, true
	}
	return p.variantFromObject(obj, KindType)
}

// resolveContract resolves the names of the //effectgo:errors directives in
// dirs. Unresolvable names are reported at pos.
func (p *pkgState) resolveContract(dirs []directive, pos token.Pos, owner string) []Variant {
	allowed := []Variant{}
	for _, d := range dirs {
		if d.kind != "errors" {
			continue
		}
		for _, name := range splitList(d.arg) {
			v, ok := p.contractVariant(name, pos)
			if !ok {
				p.pass.Reportf(pos, "effectgo:errors: cannot resolve %q in %s", name, owner)
				continue
			}
			allowed = append(allowed, v)
		}
	}
	return allowed
}

// allowedBy reports whether variant v is permitted by a contract. Unresolved
// parameters are always allowed. The contract "?" allows every opaque variant
// (including collapsed foreign ones) and "new" allows every fresh variant; the
// two do not allow each other. An abstract iface:J contract allows a variant
// whose method set includes J's methods; fresh and opaque variants have none.
func allowedBy(v Variant, allowed []Variant) bool {
	if v.Kind == KindParam {
		return true
	}
	for _, a := range allowed {
		if a.Key == v.Key {
			return true
		}
		if a.Kind == KindOpaque && v.Kind == KindOpaque {
			return true
		}
		if a.Kind == KindFresh && v.Kind == KindFresh {
			return true
		}
		if a.Kind == KindIface && v.Kind != KindOpaque && v.Kind != KindFresh && implementsAll(v.Methods, a.Methods) && len(v.Methods) > 0 {
			return true
		}
	}
	return false
}

// visible returns the variants of a set as the report, the contracts and the
// handler check show them: see collapseForeign. The module is the one of the
// package being analyzed (pass.Module); without it nothing is collapsed.
func (p *pkgState) visible(vs []Variant) []Variant {
	module := ""
	if p.pass.Module != nil {
		module = p.pass.Module.Path
	}
	return collapseForeign(vs, module)
}

func (p *pkgState) checkDirectives(u *fnUnit) {
	var dirs []directive
	for _, d := range directivesOf(u.decl.Doc) {
		switch {
		case d.kind == "errors" || d.kind == "errmap" || d.kind == "handler":
			dirs = append(dirs, d)
		case effectsKinds[d.kind]:
			// a contract of the effects analyzer, which checks it
		default:
			p.pass.Reportf(u.decl.Name.Pos(), "unknown directive //effectgo:%s", d.kind)
		}
	}

	hasContract := false
	for _, d := range dirs {
		if d.kind == "errors" {
			hasContract = true
		}
	}
	if hasContract {
		allowed := p.resolveContract(dirs, u.decl.Name.Pos(), u.obj.Name())
		for _, v := range displayOrder(p.visible(p.work[u.obj].effective().list())) {
			if allowedBy(v, allowed) {
				continue
			}
			p.pass.Reportf(u.decl.Name.Pos(), "%s may return %s%s, not in its //effectgo:errors contract", funcName(u.obj), v.Name, via(v.Chain))
		}
	}

	for _, d := range dirs {
		if d.kind == "handler" {
			p.checkHandler(u, d)
		}
	}
}

// exportErrmap validates an errmap function and exports its ErrmapFact.
// It returns the fact, or nil when the signature is wrong.
func (p *pkgState) exportErrmap(u *fnUnit) *ErrmapFact {
	sig := u.sig
	if sig.Params().Len() != 1 || sig.Results().Len() != 1 ||
		!types.Identical(sig.Params().At(0).Type(), errorType) ||
		!types.Identical(sig.Results().At(0).Type(), errorType) {
		p.pass.Reportf(u.decl.Name.Pos(), "effectgo:errmap: %s must have signature func(error) error", funcName(u.obj))
		return nil
	}
	fact := p.buildErrmap(u)
	p.pass.ExportObjectFact(u.obj, fact)
	return fact
}

// buildErrmap reads the mapping rules of a func(error) error: the variants it
// matches, the interfaces it targets, and its branches with their statuses. It
// reports nothing and exports nothing; exportErrmap does both for a directive.
func (p *pkgState) buildErrmap(u *fnUnit) *ErrmapFact {
	mapped := map[string]bool{}
	ifaces := map[string]Iface{}
	addType := func(t types.Type) {
		t = types.Unalias(t)
		if ptr, ok := t.(*types.Pointer); ok {
			t = types.Unalias(ptr.Elem())
		}
		if types.IsInterface(t) {
			var key, name string
			if n, ok := t.(*types.Named); ok {
				key, name = objKey(n.Obj()), objDisplay(n.Obj())
			}
			ifaces[key+"|"+strings.Join(methodsOf(t), ";")] = Iface{Key: key, Name: name, Methods: methodsOf(t)}
			return
		}
		if n, ok := t.(*types.Named); ok && implementsError(n) {
			mapped[objKey(n.Obj())] = true
		}
	}
	sentinelOf := func(e ast.Expr) (string, bool) {
		id, ok := unparen(e).(*ast.Ident)
		var obj types.Object
		if ok {
			obj = p.pass.TypesInfo.Uses[id]
		} else if sel, ok := unparen(e).(*ast.SelectorExpr); ok {
			obj = p.pass.TypesInfo.Uses[sel.Sel]
		}
		if v, ok := sentinelKey(obj); ok {
			return objKey(v), true
		}
		return "", false
	}

	ast.Inspect(u.decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			f := typeutil.StaticCallee(p.pass.TypesInfo, x)
			if f == nil || f.Pkg() == nil || f.Pkg().Path() != "errors" || len(x.Args) != 2 {
				return true
			}
			switch f.Name() {
			case "Is":
				if k, ok := sentinelOf(x.Args[1]); ok {
					mapped[k] = true
				}
			case "As":
				if u, ok := unparen(x.Args[1]).(*ast.UnaryExpr); ok && u.Op == token.AND {
					if t := p.pass.TypesInfo.TypeOf(u.X); t != nil {
						addType(t)
					}
				}
			}
		case *ast.BinaryExpr:
			if x.Op == token.EQL || x.Op == token.NEQ {
				if k, ok := sentinelOf(x.Y); ok {
					mapped[k] = true
				}
				if k, ok := sentinelOf(x.X); ok {
					mapped[k] = true
				}
			}
		case *ast.SwitchStmt:
			for _, s := range x.Body.List {
				cc := s.(*ast.CaseClause)
				for _, e := range cc.List {
					if k, ok := sentinelOf(e); ok {
						mapped[k] = true
					}
				}
			}
		case *ast.TypeSwitchStmt:
			for _, s := range x.Body.List {
				cc := s.(*ast.CaseClause)
				for _, e := range cc.List {
					if t := p.pass.TypesInfo.TypeOf(e); t != nil && (implementsError(t) || types.IsInterface(t)) {
						addType(t)
					}
				}
			}
		}
		return true
	})

	fact := &ErrmapFact{Branches: p.errmapBranches(u)}
	for k := range mapped {
		fact.Mapped = append(fact.Mapped, k)
	}
	sort.Strings(fact.Mapped)
	keys := make([]string, 0, len(ifaces))
	for k := range ifaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fact.Ifaces = append(fact.Ifaces, ifaces[k])
	}
	return fact
}

// handlerRef returns the argument of the //effectgo:handler directive of u
// and the errmap function it names (nil when it does not resolve).
func (p *pkgState) handlerRef(u *fnUnit) (string, *types.Func) {
	d, ok := handlerOf(u)
	if !ok {
		return "", nil
	}
	f, _ := p.resolve(d.arg, u.decl.Pos()).(*types.Func)
	return d.arg, f
}

// literalSets returns the error set of every func literal with an error result
// in the package. A literal's returns are read in the flow context of the
// declared function that encloses it, so its locals are the enclosing ones.
func (p *pkgState) literalSets() map[*ast.FuncLit]FuncErrors {
	out := map[*ast.FuncLit]FuncErrors{}
	defer func() {
		// The SSA engine's literal sets replace the AST ones where it ran.
		for lit, fe := range p.litSets {
			out[lit] = fe
		}
	}()
	for _, u := range p.units {
		c := p.newFlow(u)
		ast.Inspect(u.decl.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.FuncLit)
			if !ok {
				return true
			}
			ls, ok := c.info.TypeOf(lit).(*types.Signature)
			if ok && errIndex(ls) >= 0 {
				out[lit] = FuncErrors{Variants: p.visible(c.returnsErr(ls, lit.Body).list())}
			}
			return true
		})
	}
	return out
}

// covered reports whether errmap m maps variant v. Opaque, fresh and unresolved
// parameter variants are never covered: no errors.Is or errors.As matches them.
func covered(v Variant, m *ErrmapFact) bool {
	if v.Kind == KindOpaque || v.Kind == KindParam || v.Kind == KindFresh {
		return false
	}
	for _, k := range m.Mapped {
		if k == v.Key {
			return true
		}
	}
	for _, i := range m.Ifaces {
		if i.Key != "" && v.Key == ifacePrefix+i.Key {
			return true
		}
		if v.Kind != KindOpaque && implementsAll(v.Methods, i.Methods) {
			return true
		}
	}
	return false
}

func implementsAll(have, need []string) bool {
	set := make(map[string]bool, len(have))
	for _, h := range have {
		set[h] = true
	}
	for _, n := range need {
		if !set[n] {
			return false
		}
	}
	return true
}

func (p *pkgState) checkHandler(u *fnUnit, d directive) {
	obj := p.resolve(d.arg, u.decl.Pos())
	if obj == nil {
		p.pass.Reportf(u.decl.Name.Pos(), "effectgo:handler: cannot resolve %q", d.arg)
		return
	}
	f, ok := obj.(*types.Func)
	if !ok {
		p.pass.Reportf(u.decl.Name.Pos(), "effectgo:handler: %q is not a function", d.arg)
		return
	}
	var m ErrmapFact
	if !p.pass.ImportObjectFact(f, &m) {
		p.pass.Reportf(u.decl.Name.Pos(), "effectgo:handler: %s is not a //effectgo:errmap function", d.arg)
		return
	}
	mname := objDisplay(f)
	for _, v := range displayOrder(p.visible(p.work[u.obj].effective().list())) {
		if covered(v, &m) {
			continue
		}
		switch v.Kind {
		case KindOpaque:
			if ignoreOpaqueFlag {
				continue
			}
			p.pass.Reportf(u.decl.Name.Pos(), "handler %s may return an unclassified error%s, which errmap %s does not map → HTTP 500", funcName(u.obj), via(v.Chain), mname)
		case KindFresh:
			if ignoreOpaqueFlag {
				continue
			}
			p.pass.Reportf(u.decl.Name.Pos(), "handler %s may return fresh error %s%s, which errmap %s does not map (errors.Is and errors.As cannot match it) → HTTP 500", funcName(u.obj), v.Name, via(v.Chain), mname)
		case KindParam:
			if ignoreOpaqueFlag {
				continue
			}
			p.pass.Reportf(u.decl.Name.Pos(), "handler %s may return an unresolved %s error%s, which errmap %s does not map → HTTP 500", funcName(u.obj), v.Name, via(v.Chain), mname)
		default:
			p.pass.Reportf(u.decl.Name.Pos(), "handler %s may return %s%s, which errmap %s does not map → HTTP 500", funcName(u.obj), v.Name, via(v.Chain), mname)
		}
	}
}
