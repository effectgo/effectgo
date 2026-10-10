// Package effects is a go/analysis analyzer that infers the effects (I/O,
// time, randomness, ...) that each declared Go function may perform, both
// transitively within a package and across packages through analysis facts,
// and checks //effectgo: contracts written in doc comments.
//
// Interface contracts (directives on interface methods), effect polymorphism
// for func-typed parameters (the placeholder "param:i"), and effect stub files
// (see stubs.go) are part of the analysis.
//
// See docs/design/ and docs/reference/effects.md for the design, the fact
// format and the known limitations.
package effects

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/faintghost/effectgo/internal/dirparse"
	"github.com/faintghost/effectgo/internal/goroot"
	"github.com/faintghost/effectgo/internal/hidden"
	"github.com/faintghost/effectgo/internal/stubs"
)

// Effect names. These are the complete vocabulary accepted by //effectgo:allow
// and by stub files.
const (
	EffNet    = "net"
	EffFS     = "fs"
	EffExec   = "exec"
	EffEnv    = "env"
	EffTime   = "time"
	EffRand   = "rand"
	EffStdio  = "stdio"
	EffDB     = "db"
	EffSpawn  = "spawn"
	EffPanic  = "panic"
	EffExit   = "exit"
	EffGlobal = "global"
	EffUnkn   = "unknown"
)

var knownEffect = map[string]bool{
	EffNet: true, EffFS: true, EffExec: true, EffEnv: true, EffTime: true,
	EffRand: true, EffStdio: true, EffDB: true, EffSpawn: true, EffPanic: true,
	EffExit: true, EffGlobal: true, EffUnkn: true,
}

// paramPrefix starts the placeholder effect of a call through a func-typed
// parameter: "param:1" is a call through the parameter with index 1.
const paramPrefix = "param:"

func paramKey(i int) string { return paramPrefix + strconv.Itoa(i) }

// paramIndexOf reports whether eff is a placeholder and returns its index.
func paramIndexOf(eff string) (int, bool) {
	rest, ok := strings.CutPrefix(eff, paramPrefix)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(rest)
	return i, err == nil
}

// Step is one element of a witness chain.
type Step struct {
	Pkg  string // import path; "" for steps that are not tied to a package
	Qual string // package name, used when the step is shown from another package
	Name string // display name: "Normalize", "(*Client).Do", "Writer.Write", "counter", ...
}

// Witness is a chain of steps from a declared function down to the source of
// an effect. Witness[0] is the function the fact belongs to; the last element
// is the root (a taxonomy call, a go statement, a global variable, ...).
type Witness []Step

// EffectFact is exported on every declared *types.Func of an analyzed package.
// Effects maps each effect name the function may perform to the shortest
// witness chain found for it (first found on ties). Keys of the form
// "param:i" are placeholders for calls through the i-th parameter. The type
// is gob-encodable.
type EffectFact struct {
	Effects map[string]Witness
	// Violated lists, sorted, the effects of Effects that the function's own
	// contract does not allow. A caller that reaches one of them only through
	// this function is not reported (see suppressed): the root is here.
	Violated []string
}

// AFact marks EffectFact as an analysis.Fact.
func (*EffectFact) AFact() {}

// ContractFact is exported on an interface method that carries a contract.
// Allowed is sorted; an empty list means the method must be pure.
type ContractFact struct {
	Allowed []string
}

// AFact marks ContractFact as an analysis.Fact.
func (*ContractFact) AFact() {}

// allows reports whether a contract with this allowed list admits eff.
// Placeholders are satisfied: the function only calls its own parameter.
func (cf *ContractFact) allows(eff string) bool {
	if _, ok := paramIndexOf(eff); ok {
		return true
	}
	return slices.Contains(cf.Allowed, eff)
}

// Result is the ResultType of the effects analyzer: for every declared function
// of the package, its effect names, sorted. Placeholder keys ("param:i") are
// included. The signature analyzer reads it.
type Result map[*types.Func][]string

// Analyzer is the effects analyzer.
var Analyzer = &analysis.Analyzer{
	Name:       "effects",
	Doc:        "infers the effects of Go functions transitively and checks //effectgo: contracts",
	Run:        run,
	FactTypes:  []analysis.Fact{new(EffectFact), new(ContractFact)},
	ResultType: reflect.TypeOf(Result{}),
}

// reportFlag backs -report (-effects.report under go vet); witnessFlag backs
// -witness (-effects.witness under go vet); allFlag backs -all (-effects.all
// under go vet), which reports the violations that suppressed would hide.
var (
	reportFlag  bool
	witnessFlag bool
	allFlag     bool
)

func init() {
	Analyzer.Flags.BoolVar(&reportFlag, "report", false,
		"also emit an informational diagnostic with the inferred effect set of every declared function")
	Analyzer.Flags.BoolVar(&witnessFlag, "witness", false,
		"also emit one \"witness of <F>: <effect> via <chain>\" diagnostic per effect of every declared function")
	Analyzer.Flags.BoolVar(&allFlag, "all", false,
		"report every contract violation, including the callers whose root is another function's violation (see docs/reference/analyzers.md)")
}

// ---------------------------------------------------------------------------
// Directives and contracts

const directivePrefix = "//effectgo:"

// contract is one constraint: the set of effects a function may have.
type contract struct {
	text    string // as shown in diagnostics, e.g. "//effectgo:pure"
	allowed map[string]bool
}

func (c *contract) allows(eff string) bool {
	if _, ok := paramIndexOf(eff); ok {
		return true
	}
	return c.allowed[eff]
}

// contractText renders an interface contract the way diagnostics show it:
// "[pure]" or "[allow db,time]".
func contractText(allowed []string) string {
	if len(allowed) == 0 {
		return "[pure]"
	}
	return "[allow " + strings.Join(allowed, ",") + "]"
}

// parseDirective parses one comment. It returns isPkg for a valid
// //effectgo:package directive, the contract (nil when the comment is not a
// valid directive) and error messages. Comments that are not effectgo
// directives yield (false, nil, nil). A trailing "//" comment on a directive
// line is ignored, so tests can attach // want comments to it.
func parseDirective(c *ast.Comment, inPkgDoc bool) (bool, *contract, []string) {
	body, ok := strings.CutPrefix(c.Text, directivePrefix)
	if !ok {
		return false, nil, nil
	}
	toks := dirparse.Words(body)
	if len(toks) == 0 {
		return false, nil, []string{"empty //effectgo: directive"}
	}
	kw, rest := toks[0], toks[1:]
	if errsetKinds[kw] || otherKinds[kw] {
		// The errset analyzer reads these from the same prefix. They are not
		// effects directives, so they are neither valid nor an error here.
		return false, nil, nil
	}
	isPkg := false
	prefix := directivePrefix
	if kw == "package" {
		if !inPkgDoc {
			return false, nil, []string{"//effectgo:package is only valid in a package doc comment"}
		}
		isPkg = true
		prefix += "package "
		if len(rest) == 0 {
			return true, nil, []string{`//effectgo:package needs "pure" or "allow <effects>"`}
		}
		kw, rest = rest[0], rest[1:]
	} else if inPkgDoc {
		return false, nil, []string{fmt.Sprintf("//effectgo:%s is not valid in a package doc comment; use //effectgo:package", kw)}
	}
	switch kw {
	case "pure":
		var errs []string
		if len(rest) > 0 {
			errs = append(errs, "//effectgo:pure takes no arguments")
		}
		return isPkg, &contract{text: prefix + "pure", allowed: map[string]bool{}}, errs
	case "allow":
		names := splitNames(rest)
		if len(names) == 0 {
			return isPkg, nil, []string{"//effectgo:allow needs at least one effect name"}
		}
		var errs []string
		con := &contract{allowed: map[string]bool{}}
		for _, n := range names {
			if !knownEffect[n] {
				errs = append(errs, fmt.Sprintf("unknown effect name %q in //effectgo:allow", n))
				continue
			}
			con.allowed[n] = true
		}
		con.text = prefix + "allow " + strings.Join(names, ",")
		return isPkg, con, errs
	default:
		return isPkg, nil, []string{fmt.Sprintf("unknown effectgo directive %q", kw)}
	}
}

// errsetKinds are the directive keywords of the errset analyzer. Both
// analyzers read the //effectgo: prefix; each skips the other's keywords.
var errsetKinds = map[string]bool{"errors": true, "errmap": true, "handler": true}

// otherKinds are the directive keywords of the mustuse and exhaustive analyzers,
// which read the same prefix on types.
var otherKinds = map[string]bool{"mustuse": true, "sealed": true}

// splitNames splits "time,db" / "time db" style argument lists.
func splitNames(toks []string) []string {
	var out []string
	for _, t := range toks {
		for _, n := range strings.Split(t, ",") {
			if n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// parsePackageContract reads //effectgo:package directives from package doc
// comments. The first valid one wins; conflicting ones are reported.
func parsePackageContract(pass *analysis.Pass) *contract {
	var first *contract
	for _, f := range pass.Files {
		if f.Doc == nil {
			continue
		}
		for _, c := range f.Doc.List {
			isPkg, con, errs := parseDirective(c, true)
			for _, e := range errs {
				pass.Reportf(c.Pos(), "%s", e)
			}
			if !isPkg || con == nil {
				continue
			}
			if first == nil {
				first = con
			} else if first.text != con.text {
				pass.Reportf(c.Pos(), "conflicting package directive %q; keeping %q", con.text, first.text)
			}
		}
	}
	return first
}

// parseContracts reads the effectgo directives of one doc comment group.
// Invalid directives are reported and dropped.
func (c *checker) parseContracts(cg *ast.CommentGroup) []*contract {
	if cg == nil {
		return nil
	}
	var out []*contract
	for _, cm := range cg.List {
		_, con, errs := parseDirective(cm, false)
		for _, e := range errs {
			c.pass.Reportf(cm.Pos(), "%s", e)
		}
		if con != nil {
			out = append(out, con)
		}
	}
	return out
}

// funcContracts returns the constraints that apply to a declared function:
// its own directives if it has any, otherwise the package default. A function
// declared in a _test.go file takes no package default: a test's table of
// func values is test data, and the package contract is for the code under
// test. Its own directives still apply.
func (c *checker) funcContracts(fd *ast.FuncDecl, pkgCon *contract) []*contract {
	if own := c.parseContracts(fd.Doc); len(own) > 0 {
		return own
	}
	if pkgCon != nil && !isTestFile(c.pass, fd.Pos()) {
		return []*contract{pkgCon}
	}
	return nil
}

// isTestFile reports whether pos lies in a _test.go file.
func isTestFile(pass *analysis.Pass, pos token.Pos) bool {
	return strings.HasSuffix(pass.Fset.Position(pos).Filename, "_test.go")
}

// exportInterfaceContracts reads the directives on interface types and their
// methods. A method-level directive overrides the type-level one; several
// directives on the same declaration intersect. Each contracted method gets a
// ContractFact, which is exported so importers can use it.
func (c *checker) exportInterfaceContracts() {
	for _, f := range c.pass.Files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, sp := range gd.Specs {
				ts := sp.(*ast.TypeSpec)
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				tdoc := ts.Doc
				if tdoc == nil && len(gd.Specs) == 1 {
					tdoc = gd.Doc
				}
				typeCons := c.parseContracts(tdoc)
				for _, fld := range it.Methods.List {
					if len(fld.Names) == 0 {
						continue // embedded interface: its methods carry their own facts
					}
					cons := c.parseContracts(fld.Doc)
					if len(cons) == 0 {
						cons = typeCons
					}
					if len(cons) == 0 {
						continue
					}
					fn, _ := c.pass.TypesInfo.Defs[fld.Names[0]].(*types.Func)
					if fn == nil {
						continue
					}
					cf := &ContractFact{Allowed: intersectAllowed(cons)}
					c.pass.ExportObjectFact(fn, cf)
					c.contracts[fn] = cf
				}
			}
		}
	}
}

// intersectAllowed returns the sorted effects allowed by every contract.
func intersectAllowed(cons []*contract) []string {
	set := map[string]bool{}
	for k := range cons[0].allowed {
		set[k] = true
	}
	for _, con := range cons[1:] {
		for k := range set {
			if !con.allowed[k] {
				delete(set, k)
			}
		}
	}
	return sortedKeys(set)
}

// ---------------------------------------------------------------------------
// Per-function state

type fnState struct {
	obj     *types.Func
	decl    *ast.FuncDecl
	self    Step
	display string
	effs    map[string]Witness
	// direct holds the effects that the body of the function produces itself
	// (taxonomy calls, stubs, interface and dynamic calls, globals, ...), as
	// opposed to those that come from a call to a declared function (sites).
	direct map[string]bool
	sites  []callSite         // calls to declared, non-stub functions, resolved in the fixed point
	params map[*types.Var]int // parameters of this function, by object
}

// callSite is a call whose callee's effects are resolved in the fixed point,
// so that placeholders can be substituted with the call's arguments. A site
// with a nil callee is a call through a func value: it only charges the func
// literal arguments, since the callee is not known.
type callSite struct {
	callee *types.Func
	call   *ast.CallExpr
	offset int // 1 for a method expression call T.M(recv, args...)
}

// offer records eff with witness full (which starts with st.self) if it is
// new or strictly shorter than the current witness. It reports whether the
// state changed.
func (st *fnState) offer(eff string, full Witness) bool {
	if cur, ok := st.effs[eff]; ok && len(cur) <= len(full) {
		return false
	}
	st.effs[eff] = full
	return true
}

// hit records a direct effect whose root is step.
func (st *fnState) hit(eff string, step Step) {
	st.offer(eff, Witness{st.self, step})
}

// prepend returns [self] + tail as a fresh slice.
func prepend(self Step, tail Witness) Witness {
	w := make(Witness, 0, len(tail)+1)
	w = append(w, self)
	return append(w, tail...)
}

// splice substitutes a placeholder. w is the callee's witness for a
// "param:i" effect: its last step is the dynamic call of the parameter. The
// result is [self] + w without that last step + tail, where tail is the
// witness of the argument that replaces the parameter.
func splice(self Step, w Witness, tail Witness) Witness {
	if len(w) < 2 {
		return prepend(self, tail)
	}
	out := make(Witness, 0, len(w)+len(tail))
	out = append(out, self)
	out = append(out, w[:len(w)-1]...)
	return append(out, tail...)
}

// dynStep is the witness step of a call through a func value.
func dynStep(e ast.Expr) Step {
	return Step{Name: "dynamic call " + types.ExprString(e)}
}

// ---------------------------------------------------------------------------
// Analysis

// checker holds the state of one pass.
type checker struct {
	pass      *analysis.Pass
	stubs     *stubTable
	byObj     map[*types.Func]*fnState
	contracts map[*types.Func]*ContractFact // contracts of interface methods declared in this package
	lits      map[*ast.FuncLit]*fnState     // states of func literals that are not run by their function
	lf        *localFacts                   // local variables, for stored-value items (see via.go)
	litStates []*fnState
	// viol holds, for each declared function of this package, the effects that
	// its contract does not allow; suppressed reads it.
	viol map[*types.Func]map[string]bool
	// showAll is -all: nothing is suppressed.
	showAll bool
}

func run(pass *analysis.Pass) (any, error) {
	if len(pass.Files) == 0 || isStdlib(pass) {
		// The stdlib is modeled only by the root taxonomy and stubs, never
		// analyzed. A package without files (unsafe) has nothing to analyze.
		// The result must still have the declared type (Result), not nil.
		return Result{}, nil
	}
	rules, err := stubs.RulesForPass(pass)
	if err != nil {
		return nil, err
	}
	table, err := loadStubs(rules)
	if err != nil {
		return nil, err
	}
	c := &checker{
		pass:      pass,
		stubs:     table,
		byObj:     map[*types.Func]*fnState{},
		contracts: map[*types.Func]*ContractFact{},
		lits:      map[*ast.FuncLit]*fnState{},
		viol:      map[*types.Func]map[string]bool{},
		showAll:   allFlag,
	}
	pkgCon := parsePackageContract(pass)
	c.exportInterfaceContracts()

	var states []*fnState
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			obj, _ := pass.TypesInfo.Defs[fd.Name].(*types.Func)
			if obj == nil {
				continue
			}
			sig := obj.Type().(*types.Signature)
			params := map[*types.Var]int{}
			for i := 0; i < sig.Params().Len(); i++ {
				params[sig.Params().At(i)] = i
			}
			st := &fnState{
				obj:     obj,
				decl:    fd,
				self:    stepOf(obj),
				display: funcDisplay(obj),
				effs:    map[string]Witness{},
				params:  params,
			}
			states = append(states, st)
			c.byObj[obj] = st
		}
	}

	for _, st := range states {
		c.collect(st)
	}
	// States of the func literals that are not run by their function. They
	// take part in the fixed point, so that their effects reach the callee
	// they are passed to.
	all := append(slices.Clone(states), c.litStates...)

	// Fixed point over the call sites. Every change strictly shortens some
	// witness or adds a new effect key, so this terminates; recursion is handled.
	for changed := true; changed; {
		changed = false
		for _, st := range all {
			for _, site := range st.sites {
				if c.applySite(st, site) {
					changed = true
				}
			}
		}
	}

	// Every violation is known before any is reported: suppressed asks the
	// callees whether they violate their own contracts.
	cons := map[*fnState][]*contract{}
	for _, st := range states {
		cons[st] = c.funcContracts(st.decl, pkgCon)
		var bad []string
		for _, e := range sortedKeys(st.effs) {
			for _, con := range cons[st] {
				if !con.allows(e) {
					bad = append(bad, e)
					break
				}
			}
		}
		if len(bad) > 0 {
			c.viol[st.obj] = map[string]bool{}
			for _, e := range bad {
				c.viol[st.obj][e] = true
			}
		}
		pass.ExportObjectFact(st.obj, &EffectFact{Effects: st.effs, Violated: bad})
	}

	pkgPath := pass.Pkg.Path()
	for _, st := range states {
		effs := sortedKeys(st.effs)
		for _, e := range effs {
			for _, con := range cons[st] {
				if !con.allows(e) {
					if !c.showAll && c.suppressed(st, e) {
						hidden.Add("effects", pass.Pkg, fmt.Sprintf("%s|%s|%s", pass.Fset.Position(st.decl.Name.Pos()), st.display, e))
						break
					}
					pass.Reportf(st.decl.Name.Pos(), "%s is declared %s but has effect %s via %s",
						st.display, con.text, e, renderWitness(st.effs[e], pkgPath))
					break
				}
			}
		}
		if reportFlag {
			pass.Reportf(st.decl.Name.Pos(), "effects of %s: {%s}", st.display, strings.Join(effs, ", "))
		}
		if witnessFlag {
			for _, e := range effs {
				// The witness starts with the function itself; the chain does not.
				chain := "(direct)"
				if w := st.effs[e]; len(w) > 1 {
					chain = renderWitness(w[1:], pkgPath)
				}
				pass.Reportf(st.decl.Name.Pos(), "witness of %s: %s via %s", st.display, e, chain)
			}
		}
	}

	c.checkConformance()

	result := Result{}
	for _, st := range states {
		result[st.obj] = sortedKeys(st.effs)
	}
	return result, nil
}

// collect walks a declared function's body and records its direct effects and
// its call sites. A func literal is charged to the function only when the
// function runs it (see planLits); otherwise its body becomes a state of its
// own, reached only through the parameter it is passed to (see argHits and
// applySite).
func (c *checker) collect(st *fnState) {
	if st.decl.Body == nil {
		return
	}
	plan := planLits(c.pass.TypesInfo, st.decl.Body)
	c.walk(st, st.decl.Body, plan)
	st.direct = map[string]bool{}
	for e := range st.effs {
		st.direct[e] = true
	}
}

// walk records the direct effects and call sites of body into st. A func
// literal that the plan marks as run is walked into st; any other literal gets
// a state of its own, recorded in c.lits.
func (c *checker) walk(st *fnState, body ast.Node, plan *litPlan) {
	info := c.pass.TypesInfo
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			if !plan.run[n] {
				c.litState(st, n, plan)
				return false
			}
		case *ast.GoStmt:
			st.hit(EffSpawn, Step{Name: "go statement"})
		case *ast.CallExpr:
			c.collectCall(st, n, plan)
		case *ast.Ident:
			if v := pkgVar(info.Uses[n]); v != nil && v.Pkg().Path() == "os" {
				switch v.Name() {
				case "Args":
					st.hit(EffEnv, pkgStep(v.Pkg(), v.Name()))
				case "Stdin", "Stdout", "Stderr":
					st.hit(EffStdio, pkgStep(v.Pkg(), v.Name()))
				}
			}
		case *ast.AssignStmt:
			if n.Tok != token.DEFINE {
				for _, l := range n.Lhs {
					writeTo(info, st, l)
				}
			}
		case *ast.IncDecStmt:
			writeTo(info, st, n.X)
		case *ast.RangeStmt:
			if n.Tok == token.ASSIGN {
				if n.Key != nil {
					writeTo(info, st, n.Key)
				}
				if n.Value != nil {
					writeTo(info, st, n.Value)
				}
			}
		}
		return true
	})
}

// litState creates the state of a func literal that the enclosing function
// does not run. Its parameters are the enclosing function's (a call through
// one of them from the literal is still the enclosing function's placeholder);
// its own parameters are not placeholders, so calls through them are unknown,
// as they were before.
func (c *checker) litState(st *fnState, lit *ast.FuncLit, plan *litPlan) {
	if _, ok := c.lits[lit]; ok {
		return
	}
	pos := c.pass.Fset.Position(lit.Pos())
	ls := &fnState{
		self:    Step{Name: fmt.Sprintf("func literal (%s:%d)", filepath.Base(pos.Filename), pos.Line)},
		display: st.display + ".func",
		effs:    map[string]Witness{},
		params:  st.params,
	}
	if st.obj != nil && st.obj.Pkg() != nil {
		ls.self.Pkg, ls.self.Qual = st.obj.Pkg().Path(), st.obj.Pkg().Name()
	}
	c.lits[lit] = ls
	c.litStates = append(c.litStates, ls)
	c.walk(ls, lit.Body, plan)
}

// litPlan says which func literals of one declared function run inside it.
type litPlan struct {
	// run is the set of literals executed by the declared function (directly,
	// through defer or go, or through a local variable that it calls). Their
	// bodies are charged to the function.
	run map[*ast.FuncLit]bool
	// calls are the calls of a local variable bound to a run literal. The
	// literal's body is already charged, so the call itself is not unknown.
	calls map[*ast.CallExpr]bool
}

// planLits classifies the func literals of body. A literal is run when the
// code that contains it executes it:
//   - it is called in place, func(){...}(), including under defer and go;
//   - it is the only value ever assigned to a local variable, and a call of
//     that variable is made by code that runs (the declared function, or a
//     literal that is itself run).
//
// Any other literal (passed, stored, returned, sent) is not run: its body is
// charged only through the parameter it is passed to. A call of a local
// variable made inside a literal that is not run is not a run call, so it
// stays a call through a func value (unknown).
func planLits(info *types.Info, body *ast.BlockStmt) *litPlan {
	p := &litPlan{run: map[*ast.FuncLit]bool{}, calls: map[*ast.CallExpr]bool{}}
	type ownedCall struct {
		call  *ast.CallExpr
		owner *ast.FuncLit // the literal whose own code makes the call; nil for the declared function
	}
	var calls []ownedCall
	bound := map[*types.Var]*ast.FuncLit{}
	writes := map[*types.Var]int{}
	forEachOwn(body, nil, func(n ast.Node, owner *ast.FuncLit) {
		switch n := n.(type) {
		case *ast.CallExpr:
			calls = append(calls, ownedCall{n, owner})
			if lit, ok := ast.Unparen(n.Fun).(*ast.FuncLit); ok {
				p.run[lit] = true
			}
		case *ast.AssignStmt:
			for i, l := range n.Lhs {
				v := localVarOf(info, l)
				if v == nil {
					continue
				}
				writes[v]++
				if len(n.Lhs) == len(n.Rhs) {
					if lit, ok := ast.Unparen(n.Rhs[i]).(*ast.FuncLit); ok {
						bound[v] = lit
					}
				}
			}
		case *ast.ValueSpec:
			for i, id := range n.Names {
				v := localVarOf(info, id)
				if v == nil || len(n.Values) == 0 {
					continue // a declaration without a value is not a write
				}
				writes[v]++
				if len(n.Values) == len(n.Names) {
					if lit, ok := ast.Unparen(n.Values[i]).(*ast.FuncLit); ok {
						bound[v] = lit
					}
				}
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{n.Key, n.Value} {
				if v := localVarOf(info, e); v != nil {
					writes[v]++
				}
			}
		}
	})
	// A bound literal runs when a run caller calls its variable. The variable
	// must hold that literal for the whole function, so it is written once.
	runs := func(owner *ast.FuncLit) bool { return owner == nil || p.run[owner] }
	boundLit := func(c ownedCall) *ast.FuncLit {
		v := localVarOf(info, c.call.Fun)
		if v == nil || writes[v] != 1 {
			return nil
		}
		return bound[v]
	}
	for changed := true; changed; {
		changed = false
		for _, c := range calls {
			if lit := boundLit(c); lit != nil && !p.run[lit] && runs(c.owner) {
				p.run[lit] = true
				changed = true
			}
		}
	}
	for _, c := range calls {
		if lit := boundLit(c); lit != nil && p.run[lit] && runs(c.owner) {
			p.calls[c.call] = true
		}
	}
	return p
}

// forEachOwn calls f for every node under root that belongs to the code of
// owner: it does not descend into a nested func literal, which is visited with
// its own owner. The literal nodes themselves are not passed to f.
func forEachOwn(root ast.Node, owner *ast.FuncLit, f func(ast.Node, *ast.FuncLit)) {
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if lit, ok := n.(*ast.FuncLit); ok {
			forEachOwn(lit.Body, lit, f)
			return false
		}
		f(n, owner)
		return true
	})
}

// localVarOf returns the function-local variable that e names, or nil. Both a
// defining and a using identifier count.
func localVarOf(info *types.Info, e ast.Expr) *types.Var {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok {
		return nil
	}
	obj := info.Defs[id]
	if obj == nil {
		obj = info.Uses[id]
	}
	v, ok := obj.(*types.Var)
	if !ok || v.IsField() || v.Pkg() == nil || v.Parent() == v.Pkg().Scope() {
		return nil
	}
	return v
}

// hasFuncLitArg reports whether a call passes a func literal as an argument.
func hasFuncLitArg(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		if _, ok := ast.Unparen(a).(*ast.FuncLit); ok {
			return true
		}
	}
	return false
}

func (c *checker) collectCall(st *fnState, call *ast.CallExpr, plan *litPlan) {
	info := c.pass.TypesInfo
	if tv, ok := info.Types[call.Fun]; ok && tv.IsType() {
		return // conversion
	}
	if _, ok := ast.Unparen(call.Fun).(*ast.FuncLit); ok {
		return // a run literal: its body is already part of this function
	}
	if plan.calls[call] {
		return // a call of a run literal bound to a local variable
	}
	switch obj := typeutil.Callee(info, call).(type) {
	case *types.Builtin:
		if obj.Name() == "panic" {
			st.hit(EffPanic, Step{Name: "panic"})
		}
	case *types.Func:
		fn := obj.Origin()
		if c.methodOfField(st, call) {
			return // a method called on a field source value (see fieldsource.go)
		}
		if effs, ok := c.direct(fn); ok {
			for _, e := range sortedKeys(effs) {
				st.offer(e, prepend(st.self, effs[e]))
			}
			// A taxonomy or interface callee may still run a literal argument,
			// and a stub with via items performs the effects of its arguments.
			if fn.Pkg() != nil && (hasFuncLitArg(call) || c.hasVia(fn)) {
				st.sites = append(st.sites, callSite{callee: fn, call: call, offset: methodExprOffset(info, call)})
			}
			return
		}
		if fn.Pkg() != nil {
			st.sites = append(st.sites, callSite{callee: fn, call: call, offset: methodExprOffset(info, call)})
		}
	default:
		// A call through a stubbed package variable or named func type.
		if effs, ok := c.dynamicStub(info, call); ok {
			for _, e := range sortedKeys(effs) {
				st.offer(e, prepend(st.self, effs[e]))
			}
			return
		}
		// A literal argument of a call through a func value may be run by
		// the callee, which is not known: charge it to the caller.
		if hasFuncLitArg(call) {
			st.sites = append(st.sites, callSite{call: call})
		}
		// Call through a func-typed variable, parameter or field, or of a
		// call result. A call through one of this function's own parameters
		// is the placeholder param:i; anything else is unknown.
		if v := paramVar(info, call.Fun); v != nil {
			if i, ok := st.params[v]; ok {
				st.offer(paramKey(i), Witness{st.self, dynStep(call.Fun)})
				return
			}
		}
		st.hit(EffUnkn, dynStep(call.Fun))
	}
}

// methodExprOffset is 1 when call is a method expression call, whose first
// argument is the receiver and so is not a parameter of the method.
func methodExprOffset(info *types.Info, call *ast.CallExpr) int {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return 0
	}
	if s := info.Selections[sel]; s != nil && s.Kind() == types.MethodExpr {
		return 1
	}
	return 0
}

// paramVar returns the variable named by a plain identifier callee, or nil.
func paramVar(info *types.Info, fun ast.Expr) *types.Var {
	id, ok := ast.Unparen(fun).(*ast.Ident)
	if !ok {
		return nil
	}
	v, _ := info.Uses[id].(*types.Var)
	return v
}

// direct returns the effects of calling fn that do not come from analyzing a
// body: a stub, an interface method (its contract, or unknown), or the
// taxonomy. Precedence: stub > interface contract > taxonomy > unknown. It
// reports false when the effects must come from fn's fact (or from its body,
// when fn is declared in this package).
func (c *checker) direct(fn *types.Func) (map[string]Witness, bool) {
	iface := isIfaceMethod(fn)
	if effs, ok := c.stubs.lookup(targetKey(fn)); ok {
		step := stepOf(fn)
		if iface {
			step = stubStep(fn)
		}
		return witnessMap(effs, step), true
	}
	if iface {
		if cf := c.contractOf(fn); cf != nil {
			return witnessMap(cf.Allowed, contractStep(fn)), true
		}
		return map[string]Witness{EffUnkn: {ifaceStep(fn)}}, true
	}
	if effs := rootEffects(fn); len(effs) > 0 {
		return witnessMap(effs, stepOf(fn)), true
	}
	return nil, false
}

// dynamicStub returns the stub effects of a call through a value that a stub
// names. Two kinds of value are covered: a package-level variable of func
// type (stub target "pkgpath.Var"), which takes precedence, and a value of a
// named func type (stub target "(pkgpath.Type)"). A call is a dynamic call in
// the sense of collectCall, so it has no callee body to analyze.
func (c *checker) dynamicStub(info *types.Info, call *ast.CallExpr) (map[string]Witness, bool) {
	fun := ast.Unparen(call.Fun)
	var id *ast.Ident
	switch f := fun.(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		if info.Selections[f] == nil { // qualified identifier pkg.Var
			id = f.Sel
		}
	}
	if id != nil {
		if v := pkgVar(info.Uses[id]); v != nil {
			key := v.Pkg().Path() + "." + v.Name()
			if effs, ok := c.stubs.lookup(key); ok && len(c.stubs.viaOf(key)) == 0 {
				return witnessMap(effs, Step{Name: v.Name() + " [stub]", Pkg: v.Pkg().Path(), Qual: v.Pkg().Name()}), true
			}
		}
	}
	if n, ok := types.Unalias(info.TypeOf(call.Fun)).(*types.Named); ok {
		o := n.Origin().Obj()
		if o.Pkg() != nil {
			key := "(" + o.Pkg().Path() + "." + o.Name() + ")"
			if effs, ok := c.stubs.lookup(key); ok {
				return witnessMap(effs, Step{Name: "(" + o.Name() + ") call [stub]", Pkg: o.Pkg().Path(), Qual: o.Pkg().Name()}), true
			}
		}
	}
	return nil, false
}

func witnessMap(effs []string, step Step) map[string]Witness {
	out := make(map[string]Witness, len(effs))
	for _, e := range effs {
		out[e] = Witness{step}
	}
	return out
}

// calleeEffects returns the effects of calling fn, each with a witness that
// starts at fn.
func (c *checker) calleeEffects(fn *types.Func) map[string]Witness {
	if effs, ok := c.direct(fn); ok {
		return effs
	}
	if st := c.byObj[fn]; st != nil {
		return st.effs
	}
	if fn.Pkg() == nil {
		return nil
	}
	var f EffectFact
	if c.pass.ImportObjectFact(fn, &f) {
		return f.Effects
	}
	return nil
}

// suppressed reports whether the violation of st for effect e is only the
// consequence of a violation in a callee, so that it is not reported. That is
// the case when every source of e in st is a call to a declared function that
// itself violates its own contract for e: fixing the callee fixes st. The
// rule is stricter than "the witness's next hop violates": if st also has e
// from its own body, from a stub, an interface or dynamic call, or from a
// callee that does not violate, st is reported. A callee that is not
// analyzed (a dependency without facts, the stdlib) never suppresses.
func (c *checker) suppressed(st *fnState, e string) bool {
	if st.direct[e] {
		return false
	}
	if w := st.effs[e]; len(w) < 2 {
		return false
	}
	providers := 0
	for _, site := range st.sites {
		if site.callee == nil {
			return false // a call through a func value may perform e
		}
		if hasFuncLitArg(site.call) && !c.bodyKnown(site.callee) {
			return false // the literal's effects are charged to st
		}
		for _, h := range c.viaHits(site) {
			if h.eff == e {
				return false // a stub's argument performs e: a root, not a violating callee
			}
		}
		effs := c.calleeEffects(site.callee)
		for k := range effs {
			if _, ok := paramIndexOf(k); ok {
				return false // an argument may supply e
			}
		}
		if _, ok := effs[e]; !ok {
			continue
		}
		if !c.violates(site.callee, e) {
			return false
		}
		providers++
	}
	return providers > 0
}

// violates reports whether fn breaks its own contract for e. A function with
// a stub, an interface method and a function without facts never violates
// here: the stub, the contract or the missing analysis is the root.
func (c *checker) violates(fn *types.Func, e string) bool {
	if _, ok := c.stubs.lookup(targetKey(fn)); ok {
		return false
	}
	if isIfaceMethod(fn) {
		return false
	}
	if st := c.byObj[fn]; st != nil {
		return c.viol[fn][e]
	}
	var f EffectFact
	if c.pass.ImportObjectFact(fn, &f) {
		return slices.Contains(f.Violated, e)
	}
	return false
}

// contractOf returns the contract of an interface method, declared in this
// package or imported, or nil.
func (c *checker) contractOf(fn *types.Func) *ContractFact {
	if cf, ok := c.contracts[fn]; ok {
		return cf
	}
	if fn.Pkg() == nil {
		return nil
	}
	var cf ContractFact
	if c.pass.ImportObjectFact(fn, &cf) {
		return &cf
	}
	return nil
}

// applySite offers the effects of one call site to its caller. Placeholders
// of the callee are replaced by the effects of the argument.
func (c *checker) applySite(st *fnState, site callSite) bool {
	changed := false
	var effs map[string]Witness
	if site.callee != nil {
		effs = c.calleeEffects(site.callee)
	}
	for _, e := range sortedKeys(effs) {
		w := effs[e]
		if i, ok := paramIndexOf(e); ok {
			for _, h := range c.argHits(st, site, i) {
				if st.offer(h.eff, splice(st.self, w, h.tail)) {
					changed = true
				}
			}
			continue
		}
		if st.offer(e, prepend(st.self, w)) {
			changed = true
		}
	}
	for _, h := range c.viaHits(site) {
		if st.offer(h.eff, prepend(st.self, h.w)) {
			changed = true
		}
	}
	// A callee with no body information (stdlib, or a dependency without
	// facts) may run a func literal argument, and nothing says whether it
	// does. Charge the literal to the caller, as the callee might run it.
	if hasFuncLitArg(site.call) && (site.callee == nil || !c.bodyKnown(site.callee)) {
		var via Step
		if site.callee == nil {
			via = dynStep(site.call.Fun)
		} else {
			via = stepOf(site.callee)
		}
		for _, arg := range site.call.Args {
			lit, ok := ast.Unparen(arg).(*ast.FuncLit)
			if !ok || c.lits[lit] == nil {
				continue
			}
			ls := c.lits[lit]
			for _, e := range sortedKeys(ls.effs) {
				w := Witness{st.self, via}
				w = append(w, ls.effs[e]...)
				if st.offer(e, w) {
					changed = true
				}
			}
		}
	}
	return changed
}

// bodyKnown reports whether the effects of fn come from its body, a stub, or
// an interface contract, so that a func literal passed to it is handled by
// parameter polymorphism. A function without that information (a stdlib
// function, or a dependency without facts) is not known to ignore its
// argument.
func (c *checker) bodyKnown(fn *types.Func) bool {
	if c.byObj[fn] != nil {
		return true
	}
	if isIfaceMethod(fn) {
		return true
	}
	if _, ok := c.stubs.lookup(targetKey(fn)); ok {
		return true
	}
	if fn.Pkg() == nil {
		return false
	}
	var f EffectFact
	return c.pass.ImportObjectFact(fn, &f)
}

// argHit is an effect that an argument supplies for a placeholder. tail is
// the argument's witness.
type argHit struct {
	eff  string
	tail Witness
}

// argHits maps the argument passed for parameter i of the callee to effects:
//   - a func literal contributes nothing (its body is already part of st);
//   - a named func or method expression contributes its own effects;
//   - a parameter of st contributes the placeholder of st (param:j);
//   - anything else contributes unknown.
func (c *checker) argHits(st *fnState, site callSite, i int) []argHit {
	info := c.pass.TypesInfo
	idx := i + site.offset
	if idx >= len(site.call.Args) {
		return []argHit{{EffUnkn, Witness{{Name: "unknown argument (missing)"}}}}
	}
	arg := ast.Unparen(site.call.Args[idx])
	if lit, ok := arg.(*ast.FuncLit); ok {
		// The callee calls this parameter, so the literal's own effects are
		// what the call performs. A literal the callee only stores charges
		// nothing, because the callee's effects do not mention the parameter.
		ls := c.lits[lit]
		if ls == nil {
			return nil
		}
		var out []argHit
		for _, e := range sortedKeys(ls.effs) {
			out = append(out, argHit{e, ls.effs[e]})
		}
		return out
	}
	if v := paramVar(info, arg); v != nil {
		if j, ok := st.params[v]; ok {
			return []argHit{{paramKey(j), Witness{dynStep(arg)}}}
		}
	}
	fn := c.funcRef(arg)
	if fn == nil {
		return []argHit{{EffUnkn, Witness{{Name: "unknown argument " + types.ExprString(arg)}}}}
	}
	var out []argHit
	if c.hasVia(fn) {
		// Used as a value, the callee's writers and readers are not known.
		out = append(out, argHit{EffUnkn, Witness{stepOf(fn)}})
	}
	effs := c.calleeEffects(fn)
	for _, e := range sortedKeys(effs) {
		if _, ok := paramIndexOf(e); ok {
			// The argument itself takes a func parameter; it cannot be
			// substituted here, so the effect stays unknown.
			out = append(out, argHit{EffUnkn, Witness{stepOf(fn)}})
			continue
		}
		out = append(out, argHit{e, effs[e]})
	}
	return out
}

// funcRef returns the declared or stdlib function that a func-valued
// expression names, when it names one directly: an identifier, a qualified
// identifier, a method value or method expression, or an explicit
// instantiation of one of these. It returns nil otherwise.
func (c *checker) funcRef(e ast.Expr) *types.Func {
	info := c.pass.TypesInfo
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		if fn, ok := info.Uses[e].(*types.Func); ok {
			return fn.Origin()
		}
	case *ast.SelectorExpr:
		if s := info.Selections[e]; s != nil {
			if fn, ok := s.Obj().(*types.Func); ok && s.Kind() != types.FieldVal {
				return fn.Origin()
			}
			return nil
		}
		if fn, ok := info.Uses[e.Sel].(*types.Func); ok {
			return fn.Origin()
		}
	case *ast.IndexExpr:
		return c.funcRef(e.X)
	case *ast.IndexListExpr:
		return c.funcRef(e.X)
	}
	return nil
}

// writeTo records a global effect when the assignment target is (rooted at)
// a package-level variable.
func writeTo(info *types.Info, st *fnState, e ast.Expr) {
	if v := globalRoot(info, e); v != nil {
		st.hit(EffGlobal, pkgStep(v.Pkg(), v.Name()))
	}
}

// globalRoot returns the package-level variable an assignment target is
// rooted at: x, x.f, x[i], (x), pkg.X. A pointer dereference (*p) is not
// followed.
func globalRoot(info *types.Info, e ast.Expr) *types.Var {
	switch e := e.(type) {
	case *ast.Ident:
		return pkgVar(info.Uses[e])
	case *ast.ParenExpr:
		return globalRoot(info, e.X)
	case *ast.IndexExpr:
		return globalRoot(info, e.X)
	case *ast.SelectorExpr:
		if info.Selections[e] == nil { // qualified identifier pkg.X
			return pkgVar(info.Uses[e.Sel])
		}
		return globalRoot(info, e.X)
	}
	return nil
}

// pkgVar returns obj as a package-level variable, or nil.
func pkgVar(obj types.Object) *types.Var {
	v, ok := obj.(*types.Var)
	if !ok || v.Pkg() == nil || v.IsField() || v.Parent() != v.Pkg().Scope() {
		return nil
	}
	return v
}

// rootEffects is the effect taxonomy for stdlib functions and methods. It is
// the only model of the standard library, apart from stub files; no stdlib
// body is analyzed.
func rootEffects(fn *types.Func) []string {
	if fn.Pkg() == nil {
		return nil
	}
	path, name := fn.Pkg().Path(), fn.Name()
	ptr, tname, method := recvInfo(fn)

	switch path {
	case "os/exec":
		return []string{EffExec}
	case "io/ioutil":
		return []string{EffFS}
	case "log":
		out := []string{EffStdio}
		if strings.HasPrefix(name, "Fatal") {
			out = append(out, EffExit)
		}
		if strings.HasPrefix(name, "Panic") {
			out = append(out, EffPanic)
		}
		return out
	}

	if !method {
		switch path {
		case "net":
			if hasPrefixAny(name, "Dial", "Listen") {
				return []string{EffNet}
			}
		case "net/http":
			if inSet(name, "Get", "Post", "Head", "PostForm") || strings.HasPrefix(name, "ListenAndServe") {
				return []string{EffNet}
			}
		case "os":
			switch {
			case inSet(name, "Open", "OpenFile", "Create", "ReadFile", "WriteFile", "Remove", "RemoveAll",
				"Mkdir", "MkdirAll", "Rename", "Stat", "Lstat", "ReadDir", "Chdir", "Chmod"):
				return []string{EffFS}
			case inSet(name, "Getenv", "LookupEnv", "Setenv", "Unsetenv", "Environ", "Hostname", "Getwd"):
				return []string{EffEnv}
			case name == "Exit":
				return []string{EffExit}
			}
		case "time":
			if inSet(name, "Now", "Since", "Until", "Sleep", "After", "AfterFunc", "Tick", "NewTimer", "NewTicker") {
				return []string{EffTime}
			}
		case "math/rand", "math/rand/v2":
			return []string{EffRand}
		case "crypto/rand":
			if inSet(name, "Read", "Int", "Prime", "Text") {
				return []string{EffRand}
			}
		case "fmt":
			if inSet(name, "Print", "Printf", "Println") || strings.HasPrefix(name, "Scan") {
				return []string{EffStdio}
			}
		}
		return nil
	}

	switch path {
	case "os":
		if ptr && tname == "File" {
			return []string{EffFS}
		}
	case "net/http":
		if ptr && tname == "Client" && inSet(name, "Do", "Get", "Post", "Head", "PostForm") {
			return []string{EffNet}
		}
		if ptr && tname == "Server" && strings.HasPrefix(name, "ListenAndServe") {
			return []string{EffNet}
		}
	case "database/sql":
		if ptr && inSet(tname, "DB", "Tx", "Stmt", "Conn") {
			return []string{EffDB}
		}
	}
	return nil
}

func inSet(s string, set ...string) bool {
	return slices.Contains(set, s)
}

func hasPrefixAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// recvInfo describes a method receiver: whether it is a pointer, the name of
// its named type, and whether fn is a method at all.
func recvInfo(fn *types.Func) (ptr bool, tname string, method bool) {
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return false, "", false
	}
	t := types.Unalias(recv.Type())
	if p, ok := t.(*types.Pointer); ok {
		ptr = true
		t = types.Unalias(p.Elem())
	}
	if n, ok := t.(*types.Named); ok {
		tname = n.Obj().Name()
	}
	return ptr, tname, true
}

// methodOwner returns the receiver's pointer-ness and the type name that
// declares a method (nil for an unnamed receiver).
func methodOwner(fn *types.Func) (ptr bool, owner *types.TypeName) {
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return false, nil
	}
	t := types.Unalias(recv.Type())
	if p, ok := t.(*types.Pointer); ok {
		ptr = true
		t = types.Unalias(p.Elem())
	}
	if n, ok := t.(*types.Named); ok {
		owner = n.Origin().Obj()
	}
	return ptr, owner
}

// isIfaceMethod reports whether fn is declared by an interface (or a type
// parameter constraint).
func isIfaceMethod(fn *types.Func) bool {
	recv := fn.Type().(*types.Signature).Recv()
	return recv != nil && types.IsInterface(recv.Type())
}

// funcDisplay is the name used for a function in messages.
func funcDisplay(fn *types.Func) string {
	ptr, tname, method := recvInfo(fn)
	switch {
	case !method || tname == "":
		return fn.Name()
	case ptr:
		return "(*" + tname + ")." + fn.Name()
	default:
		return tname + "." + fn.Name()
	}
}

func stepOf(fn *types.Func) Step {
	s := Step{Name: funcDisplay(fn)}
	if p := fn.Pkg(); p != nil {
		s.Pkg, s.Qual = p.Path(), p.Name()
	}
	return s
}

func pkgStep(p *types.Package, name string) Step {
	return Step{Pkg: p.Path(), Qual: p.Name(), Name: name}
}

// ifaceOwnerStep returns a step named "(T).M" + tag, or "T.M" when tagged is
// empty, for an interface method, located in the package that declares T.
func ifaceOwnerStep(fn *types.Func, paren bool, tag string) Step {
	_, owner := methodOwner(fn)
	name := "(interface)"
	pkg := fn.Pkg()
	if owner != nil {
		name = owner.Name()
		if paren {
			name = "(" + name + ")"
		}
		if owner.Pkg() != nil {
			pkg = owner.Pkg()
		}
	}
	s := Step{Name: name + "." + fn.Name() + tag}
	if pkg != nil {
		s.Pkg, s.Qual = pkg.Path(), pkg.Name()
	}
	return s
}

// ifaceStep names an interface method call, e.g. io.Writer.Write.
func ifaceStep(fn *types.Func) Step { return ifaceOwnerStep(fn, false, "") }

// contractStep names a call that is resolved through an interface contract,
// e.g. "repo.(Repo).Get [contract]".
func contractStep(fn *types.Func) Step { return ifaceOwnerStep(fn, true, " [contract]") }

// stubStep names a call that is resolved through a stub, e.g. "io.(Writer).Write [stub]".
func stubStep(fn *types.Func) Step { return ifaceOwnerStep(fn, true, " [stub]") }

// renderWitness renders a chain relative to the package being analyzed:
// steps from that package are unqualified, others are qualified by package name.
func renderWitness(w Witness, cur string) string {
	parts := make([]string, len(w))
	for i, s := range w {
		if s.Pkg != "" && s.Pkg != cur && s.Qual != "" {
			parts[i] = s.Qual + "." + s.Name
		} else {
			parts[i] = s.Name
		}
	}
	return strings.Join(parts, " → ")
}

// isStdlib reports whether the package is compiled from a Go installation's
// src tree. The check reads the file paths only (see internal/goroot).
func isStdlib(pass *analysis.Pass) bool {
	names := make([]string, 0, len(pass.Files))
	for _, f := range pass.Files {
		if tf := pass.Fset.File(f.Pos()); tf != nil {
			names = append(names, tf.Name())
		}
	}
	return goroot.FilesAreStd(pass.Pkg.Path(), names)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---------------------------------------------------------------------------
// Conformance

// contractedIface is an interface, declared in this package or in a transitively
// imported one, that has at least one method with a contract.
type contractedIface struct {
	tn    *types.TypeName
	iface *types.Interface
}

// contractedInterfaces walks the current package and its transitive imports
// and returns every named interface that has a contracted method.
func (c *checker) contractedInterfaces() []contractedIface {
	var out []contractedIface
	seen := map[*types.Package]bool{}
	var visit func(p *types.Package)
	visit = func(p *types.Package) {
		if p == nil || seen[p] {
			return
		}
		seen[p] = true
		scope := p.Scope()
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			iface, ok := tn.Type().Underlying().(*types.Interface)
			if !ok {
				continue
			}
			for i := 0; i < iface.NumMethods(); i++ {
				if c.contractOf(iface.Method(i)) != nil {
					out = append(out, contractedIface{tn: tn, iface: iface})
					break
				}
			}
		}
		for _, imp := range p.Imports() {
			visit(imp)
		}
	}
	visit(c.pass.Pkg)
	return out
}

// checkConformance checks every named non-interface type declared in this
// package against every contracted interface it implements (through T or *T):
// each implementing method's effects must be allowed by the method's contract.
func (c *checker) checkConformance() {
	ifaces := c.contractedInterfaces()
	if len(ifaces) == 0 {
		return
	}
	scope := c.pass.Pkg.Scope()
	pkgPath := c.pass.Pkg.Path()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok || types.IsInterface(named) || named.TypeParams().Len() > 0 {
			continue
		}
		ptr := types.NewPointer(named)
		for _, ci := range ifaces {
			if !types.Implements(named, ci.iface) && !types.Implements(ptr, ci.iface) {
				continue
			}
			for i := 0; i < ci.iface.NumMethods(); i++ {
				m := ci.iface.Method(i)
				cf := c.contractOf(m)
				if cf == nil {
					continue
				}
				obj, _, _ := types.LookupFieldOrMethod(ptr, true, m.Pkg(), m.Name())
				impl, ok := obj.(*types.Func)
				if !ok || isIfaceMethod(impl) {
					continue
				}
				effs, pos, ok := c.implEffects(impl, tn)
				if !ok {
					continue
				}
				for _, e := range sortedKeys(effs) {
					if cf.allows(e) {
						continue
					}
					c.pass.Reportf(pos, "%s violates contract of (%s).%s %s: has effect %s via %s",
						funcDisplay(impl), ci.tn.Name(), m.Name(), contractText(cf.Allowed),
						e, renderWitness(effs[e], pkgPath))
				}
			}
		}
	}
}

// implEffects returns the inferred effects of an implementing method and the
// position to report at: the method's name when it is declared here, or the
// type's name when the method is promoted from another package.
func (c *checker) implEffects(impl *types.Func, owner *types.TypeName) (map[string]Witness, token.Pos, bool) {
	fn := impl.Origin()
	if st := c.byObj[fn]; st != nil {
		return st.effs, st.decl.Name.Pos(), true
	}
	if fn.Pkg() == nil {
		return nil, 0, false
	}
	var f EffectFact
	if c.pass.ImportObjectFact(fn, &f) {
		return f.Effects, owner.Pos(), true
	}
	return nil, 0, false
}
