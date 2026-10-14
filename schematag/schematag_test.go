package schematag

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestSchematag runs the analyzer on the testdata: the findings are the want
// comments, and every package without want comments must stay silent.
func TestSchematag(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer,
		"rtuse/good", "rtuse/tagbad", "rtuse/typebad", "rtuse/routebad", "rtuse/nonconst",
		"rtuse/enumbad", "rtuse/cross", "gorilla/plain")
}

// The analyzer cannot import rt (a separate module), so its vocabulary is copied.
// These tests read rt/schema/schema.go and fail when the copy and the source differ.

const schemaSource = "../rt/schema/schema.go"

func parseSchema(t *testing.T) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.FromSlash(schemaSource), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// funcBody returns the body of the top-level function name.
func funcBody(t *testing.T, f *ast.File, name string) *ast.BlockStmt {
	t.Helper()
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == name && fd.Recv == nil {
			return fd.Body
		}
	}
	t.Fatalf("function %s not found in %s", name, schemaSource)
	return nil
}

// firstSwitch returns the first switch statement that is a direct statement of body.
func firstSwitch(t *testing.T, body *ast.BlockStmt) *ast.SwitchStmt {
	t.Helper()
	for _, s := range body.List {
		if sw, ok := s.(*ast.SwitchStmt); ok {
			return sw
		}
	}
	t.Fatal("no switch statement")
	return nil
}

// idents collects the identifiers with the given prefix under n.
func idents(n ast.Node, prefix string) []string {
	var out []string
	ast.Inspect(n, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && strings.HasPrefix(id.Name, prefix) {
			out = append(out, id.Name)
		}
		return true
	})
	return out
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// TestRuleVocabulary: the rule names, the rules without an argument, and the field
// categories each rule applies to, as parseRule defines them.
func TestRuleVocabulary(t *testing.T) {
	sw := firstSwitch(t, funcBody(t, parseSchema(t), "parseRule"))
	got := map[string]struct {
		noArg bool
		cats  []string
	}{}
	for _, s := range sw.Body.List {
		cc := s.(*ast.CaseClause)
		var names []string
		for _, e := range cc.List {
			names = append(names, strings.Trim(e.(*ast.BasicLit).Value, `"`))
		}
		noArg := false
		var cats []string
		ast.Inspect(&ast.BlockStmt{List: cc.Body}, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "noArg" {
					noArg = true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "needString" {
					cats = append(cats, "string")
				}
			}
			return true
		})
		for _, id := range idents(&ast.BlockStmt{List: cc.Body}, "cat") {
			if len(id) > len("cat") { // cat itself is the variable of the switch
				cats = append(cats, strings.ToLower(strings.TrimPrefix(id, "cat")))
			}
		}
		cats = uniq(cats)
		for _, n := range names {
			got[n] = struct {
				noArg bool
				cats  []string
			}{noArg, sorted(cats)}
		}
	}
	want := map[string]struct {
		noArg bool
		cats  []string
	}{}
	for name, spec := range rules {
		want[name] = struct {
			noArg bool
			cats  []string
		}{spec.noArg, sorted(spec.cats)}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rule vocabulary differs from rt/schema parseRule\n got: %v\nwant: %v", got, want)
	}
}

// TestCategories: the kind-to-category mapping of categoryOf.
func TestCategories(t *testing.T) {
	sw := firstSwitch(t, funcBody(t, parseSchema(t), "categoryOf"))
	got := map[string]string{}
	for _, s := range sw.Body.List {
		cc := s.(*ast.CaseClause)
		if len(cc.Body) != 1 {
			continue
		}
		ret, ok := cc.Body[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		cat := strings.ToLower(strings.TrimPrefix(ret.Results[0].(*ast.Ident).Name, "cat"))
		if cat == "none" {
			continue
		}
		for _, e := range cc.List {
			sel := e.(*ast.SelectorExpr)
			got[sel.Sel.Name] = cat
		}
	}
	want := map[string]string{}
	for kind, cat := range kindCategory {
		want[goKindName(kind)] = cat
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("categoryOf differs from rt/schema\n got: %v\nwant: %v", got, want)
	}
}

// TestSupportedKinds: the kinds that checkType accepts (its case list, not the
// bodies).
func TestSupportedKinds(t *testing.T) {
	sw := firstSwitch(t, funcBody(t, parseSchema(t), "checkType"))
	var got []string
	for _, s := range sw.Body.List {
		for _, e := range s.(*ast.CaseClause).List {
			got = append(got, e.(*ast.SelectorExpr).Sel.Name)
		}
	}
	var want []string
	for _, k := range supportedKinds {
		want = append(want, goKindName(k))
	}
	if !reflect.DeepEqual(sorted(got), sorted(want)) {
		t.Fatalf("supported kinds differ from rt/schema checkType\n got: %v\nwant: %v", sorted(got), sorted(want))
	}
}

func uniq(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// goKindName is the name of a kind as rt/schema writes it (reflect.Pointer,
// reflect.Int8, ...).
func goKindName(k reflect.Kind) string {
	if k == reflect.Pointer {
		return "Pointer"
	}
	s := k.String()
	return strings.ToUpper(s[:1]) + s[1:]
}
