package rtshape

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// check type-checks one source file of package p and returns it.
func check(t *testing.T, src string) *types.Package {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("example.com/p", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatalf("type check: %v", err)
	}
	return pkg
}

// describe describes the type named Root of src.
func describe(t *testing.T, src string, opt Options) *Shape {
	t.Helper()
	pkg := check(t, src)
	obj := pkg.Scope().Lookup("Root")
	if obj == nil {
		t.Fatal("no type Root")
	}
	return Describe(obj.Type(), opt)
}

// field returns the field named name of the struct type s.
func field(t *testing.T, s *Type, name string) *Field {
	t.Helper()
	for _, f := range s.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no field %s", name)
	return nil
}

// messages returns the problems of s as "where: msg" strings.
func messages(s *Shape) []string {
	var out []string
	for _, p := range s.Problems {
		m := p.Msg
		if p.Where != "" {
			m = p.Where + ": " + m
		}
		out = append(out, m)
	}
	return out
}

func wantProblem(t *testing.T, s *Shape, sub string) {
	t.Helper()
	for _, m := range messages(s) {
		if strings.Contains(m, sub) {
			return
		}
	}
	t.Errorf("no problem containing %q; got %q", sub, messages(s))
}

func TestCategoryTable(t *testing.T) {
	s := describe(t, `package p
import ("time"; "encoding/json")
type Color string
const (ColorRed Color = "red"; ColorBlue Color = "blue")
func (c Color) Valid() bool { switch c { case ColorRed, ColorBlue: return true }; return false }
type Item struct{ N int }
type Root struct {
	S    string
	I    int64
	U    uint8
	F    float64
	B    bool
	T    time.Time
	R    json.RawMessage
	A    any
	L    []string
	M    map[string]int
	St   Item
	E    Color
	P    []*Item
	C    chan int
}`, Options{})
	want := map[string]Category{
		"S": CatString, "I": CatInt, "U": CatInt, "F": CatFloat, "B": CatBool,
		"T": CatTime, "R": CatRaw, "A": CatAny, "L": CatSlice, "M": CatMap,
		"St": CatStruct, "E": CatEnum, "P": CatSlice, "C": CatUnsupported,
	}
	for name, cat := range want {
		if got := field(t, s.Root, name).Type.Cat; got != cat {
			t.Errorf("field %s: category %s, want %s", name, got, cat)
		}
	}
	if got := field(t, s.Root, "P").Type.Elem.Cat; got != CatPointer {
		t.Errorf("[]*Item element: category %s, want pointer", got)
	}
	if got := field(t, s.Root, "P").Type.Elem.Elem.Cat; got != CatStruct {
		t.Errorf("[]*Item element of element: category %s, want struct", got)
	}
	if got := field(t, s.Root, "M").Type.Elem.Cat; got != CatInt {
		t.Errorf("map value: category %s, want int", got)
	}
	if got := field(t, s.Root, "U").Type.RuleCat; got != "uint" {
		t.Errorf("uint8 RuleCat %q, want uint", got)
	}
	if got := field(t, s.Root, "T").Type.RuleCat; got != "" {
		t.Errorf("time RuleCat %q, want empty", got)
	}
	wantProblem(t, s, "type chan int is not supported")
}

func TestOptionalAndOmitempty(t *testing.T) {
	s := describe(t, `package p
type Root struct {
	A string `+"`json:\"a\"`"+`
	B *string `+"`json:\"b,omitempty\"`"+`
	C **int `+"`json:\"c\"`"+`
	D int `+"`json:\",omitempty\"`"+`
	E string `+"`json:\"-\"`"+`
	F int
}`, Options{})
	a := field(t, s.Root, "A")
	if a.Optional || a.Omitempty || a.JSON != "a" {
		t.Errorf("A: optional %v omitempty %v json %q", a.Optional, a.Omitempty, a.JSON)
	}
	b := field(t, s.Root, "B")
	if !b.Optional || !b.Omitempty || b.Type.Cat != CatString {
		t.Errorf("B: optional %v omitempty %v cat %s", b.Optional, b.Omitempty, b.Type.Cat)
	}
	c := field(t, s.Root, "C")
	if !c.Optional || c.Type.Cat != CatPointer || c.Type.Elem.Cat != CatInt {
		t.Errorf("C: optional %v, inner %s", c.Optional, c.Type.Cat)
	}
	d := field(t, s.Root, "D")
	if d.Optional || !d.Omitempty || d.JSON != "D" {
		t.Errorf("D: optional %v omitempty %v json %q", d.Optional, d.Omitempty, d.JSON)
	}
	if f := field(t, s.Root, "F"); f.JSON != "F" || f.Omitempty {
		t.Errorf("F: json %q omitempty %v", f.JSON, f.Omitempty)
	}
	for _, f := range s.Root.Fields {
		if f.Name == "E" {
			t.Error("json:\"-\" field E is described")
		}
	}
}

func TestEnumConstantsInSourceOrder(t *testing.T) {
	s := describe(t, `package p
type Color int
const (
	Zeta Color = iota
	Alpha
	Mid
)
const Other = 9
const Unrelated int = 1
func (c Color) Valid() bool { switch c { case Zeta, Alpha, Mid: return true }; return false }
type Root struct{ C Color }`, Options{})
	enum := field(t, s.Root, "C").Type
	if enum.Cat != CatEnum {
		t.Fatalf("category %s, want enum", enum.Cat)
	}
	var got []string
	for _, c := range enum.Consts {
		got = append(got, c.Name())
	}
	if strings.Join(got, ",") != "Zeta,Alpha,Mid" {
		t.Errorf("constants %v, want source order Zeta,Alpha,Mid", got)
	}
	if enum.RuleCat != "int" {
		t.Errorf("enum RuleCat %q, want int", enum.RuleCat)
	}
}

func TestNestedStructOnceAndRecursion(t *testing.T) {
	s := describe(t, `package p
type Inner struct{ X int }
type Node struct {
	Next *Node
	Val  Inner
}
type Root struct {
	A Inner
	B []Inner
	N Node
	M map[string]Inner
}`, Options{})
	a := field(t, s.Root, "A").Type
	b := field(t, s.Root, "B").Type.Elem
	m := field(t, s.Root, "M").Type.Elem
	if a != b || a != m {
		t.Error("Inner reached three times is not one shared *Type")
	}
	n := field(t, s.Root, "N").Type
	if next := field(t, n, "Next").Type; next != n {
		t.Error("the recursive Next field does not point back to Node's *Type")
	}
	if n2 := field(t, n, "Val").Type; n2 != a {
		t.Error("Node.Val does not share Inner's *Type")
	}
	names := map[string]int{}
	for _, ty := range s.Types {
		names[ty.Name]++
	}
	if names["Inner"] != 1 || names["Node"] != 1 || names["Root"] != 1 {
		t.Errorf("Types has each struct once: %v", names)
	}
	if len(s.Problems) != 0 {
		t.Errorf("problems %q, want none", messages(s))
	}
}

func TestRulesParsedAndRejected(t *testing.T) {
	s := describe(t, `package p
type Root struct {
	Name  string  `+"`json:\"name\" schema:\"trim,nonempty,max=200,singleline\"`"+`
	Email *string `+"`json:\"email\" schema:\"email\"`"+`
	Age   int     `+"`json:\"age\" schema:\"email\"`"+`
	Dup   string  `+"`json:\"dup\" schema:\"trim,trim\"`"+`
	Empty string  `+"`json:\"e\" schema:\"trim,,max=1\"`"+`
	Bad   string  `+"`json:\"bad\" schema:\"max=abc\"`"+`
	Named string  `+"`json:\"named\" schema:\"pattern\"`"+`
	Unk   string  `+"`json:\"unk\" schema:\"maxlen=3\"`"+`
}`, Options{})
	name := field(t, s.Root, "Name")
	var got []string
	for _, r := range name.Rules {
		got = append(got, r.Name)
	}
	if strings.Join(got, ",") != "trim,nonempty,max,singleline" {
		t.Errorf("Name rules %v", got)
	}
	if r := name.Rules[2]; r.Arg != "200" || !r.HasArg {
		t.Errorf("max rule %+v", r)
	}
	if e := field(t, s.Root, "Email"); len(e.Rules) != 1 || !e.Optional {
		t.Errorf("Email: rules %v optional %v", e.Rules, e.Optional)
	}
	if a := field(t, s.Root, "Age"); len(a.Rules) != 0 {
		t.Errorf("Age: a rule that does not fit was kept: %v", a.Rules)
	}
	wantProblem(t, s, `rule "email" does not apply to int`)
	wantProblem(t, s, `rule "trim" is repeated`)
	wantProblem(t, s, "empty rule")
	wantProblem(t, s, `"abc" is not a length`)
	wantProblem(t, s, `rule "pattern" needs a pattern name`)
	wantProblem(t, s, `unknown rule "maxlen"`)
	if got := field(t, s.Root, "Dup").Rules; len(got) != 1 {
		t.Errorf("Dup: %d rules kept, want 1", len(got))
	}
}

func TestPathAndQueryTags(t *testing.T) {
	s := describe(t, `package p
type Root struct {
	ID   int64 `+"`path:\"id\" json:\"id\"`"+`
	Page int   `+"`query:\"page\"`"+`
}`, Options{})
	if f := field(t, s.Root, "ID"); f.Path != "id" || f.Query != "" {
		t.Errorf("ID: path %q query %q", f.Path, f.Query)
	}
	if f := field(t, s.Root, "Page"); f.Query != "page" || f.Path != "" {
		t.Errorf("Page: path %q query %q", f.Path, f.Query)
	}
}

func TestProblemsOfUnsupportedTypes(t *testing.T) {
	s := describe(t, `package p
import "fmt"
type Root struct {
	Keyed map[int]string `+"`json:\"keyed\"`"+`
	Bytes []byte         `+"`json:\"bytes\"`"+`
	Iface fmt.Stringer   `+"`json:\"iface\"`"+`
	Cplx  complex128     `+"`json:\"cplx\"`"+`
	A     string         `+"`json:\"x\"`"+`
	B     string         `+"`json:\"x\"`"+`
	hid   string         `+"`schema:\"trim\"`"+`
}`, Options{})
	wantProblem(t, s, "map key must be a string kind, got int")
	wantProblem(t, s, "byte slices are not supported")
	wantProblem(t, s, "non-empty interface fmt.Stringer is not supported")
	wantProblem(t, s, "type complex128 is not supported")
	wantProblem(t, s, `json name "x" is used by two fields of Root`)
	wantProblem(t, s, "unexported field has a schema tag")
	if len(s.Problems) != 6 {
		t.Errorf("%d problems, want 6: %q", len(s.Problems), messages(s))
	}
	for _, p := range s.Problems {
		if p.Owner == nil && p.Where != "" {
			t.Errorf("field problem %q has no owner", p.Msg)
		}
	}
}

func TestEmbeddedFieldIsAProblem(t *testing.T) {
	s := describe(t, `package p
type Inner struct{ X int }
type Root struct{ Inner }`, Options{})
	wantProblem(t, s, "embedded fields are not supported")
}

func TestSealedHookIsCalledPerVisit(t *testing.T) {
	src := `package p
type Seg string
type Root struct {
	A Seg
	B []Seg
}`
	calls := 0
	s := describe(t, src, Options{Sealed: func(n *types.Named) string {
		calls++
		if n.Obj().Name() == "Seg" {
			return "sealed Seg"
		}
		return ""
	}})
	// Root itself is a named visit, then A, then the element of B.
	if calls != 3 {
		t.Errorf("Sealed called %d times, want once per visit (3)", calls)
	}
	if len(s.Problems) != 2 {
		t.Fatalf("problems %q, want the hook message twice", messages(s))
	}
	if s.Problems[0].Owner == nil || s.Problems[0].Owner.Name() != "A" {
		t.Errorf("first hook problem owner %v, want field A", s.Problems[0].Owner)
	}
	if s.Problems[0].Msg != "sealed Seg" {
		t.Errorf("hook message %q", s.Problems[0].Msg)
	}
}

func TestRootUnsupportedAndUnknown(t *testing.T) {
	s := describe(t, `package p
type Root = chan int`, Options{})
	if s.Root.Cat != CatUnsupported || len(s.Problems) != 1 || s.Problems[0].Owner != nil {
		t.Errorf("root chan: cat %s, problems %q", s.Root.Cat, messages(s))
	}
}

func TestRuleVocabularyIsComplete(t *testing.T) {
	if got := RuleNames(); got != "email, max, min, nonempty, pattern, singleline, trim" {
		t.Errorf("RuleNames = %q", got)
	}
}
