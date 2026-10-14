package schema

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

type Segment string

func (s Segment) Valid() bool {
	switch s {
	case "hot", "nurture":
		return true
	}
	return false
}

type Level int

func (l *Level) Valid() bool { return *l >= 0 && *l <= 2 }

type Contact struct {
	ID         int64   `json:"id" schema:"min=1"`
	Name       string  `json:"name" schema:"trim,nonempty,max=5,singleline"`
	Email      string  `json:"email" schema:"email"`
	Segment    Segment `json:"segment"`
	Notes      *string `json:"notes" schema:"max=20"`
	Suppressed bool    `json:"suppressed"`
}

func TestDecodeValid(t *testing.T) {
	c, err := Decode[Contact]([]byte(`{"id":7,"name":"  Ada  ","email":"ada@example.com","segment":"hot","suppressed":true}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c.ID != 7 || c.Name != "Ada" || c.Email != "ada@example.com" || c.Segment != "hot" || !c.Suppressed || c.Notes != nil {
		t.Fatalf("got %+v", c)
	}
	c, err = Decode[Contact]([]byte(`{"id":1,"name":"A","email":"a@b.c","segment":"nurture","notes":"hi","suppressed":false} `))
	if err != nil || c.Notes == nil || *c.Notes != "hi" {
		t.Fatalf("optional pointer: %+v %v", c, err)
	}
	c, err = Decode[Contact]([]byte(`{"id":1,"name":"A","email":"a@b.c","segment":"hot","notes":null,"suppressed":false}`))
	if err != nil || c.Notes != nil {
		t.Fatalf("null pointer should be nil: %+v %v", c, err)
	}
}

func issuesOf(t *testing.T, err error) []Issue {
	t.Helper()
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("error %v (%T) is not a *DecodeError", err, err)
	}
	return de.Issues
}

func hasIssue(issues []Issue, path string, code string) bool {
	for _, is := range issues {
		if pathString(is.Path) == path && is.Code == code {
			return true
		}
	}
	return false
}

func TestDecodeCollectsAllIssues(t *testing.T) {
	_, err := Decode[Contact]([]byte(`{"id":0,"name":"   ","email":"not-an-email","segment":"cold","extra":1,"suppressed":"yes"}`))
	issues := issuesOf(t, err)
	want := []struct{ path, code string }{
		{"id", CodeMin},
		{"name", CodeNonEmpty},
		{"email", CodeEmail},
		{"segment", CodeEnum},
		{"extra", CodeUnknown},
		{"suppressed", CodeType},
	}
	for _, w := range want {
		if !hasIssue(issues, w.path, w.code) {
			t.Errorf("missing issue %s/%s in %+v", w.path, w.code, issues)
		}
	}
	if len(issues) != len(want) {
		t.Errorf("got %d issues, want %d: %+v", len(issues), len(want), issues)
	}
}

func TestDecodeMissingAndStrictTypes(t *testing.T) {
	_, err := Decode[Contact]([]byte(`{}`))
	issues := issuesOf(t, err)
	for _, p := range []string{"id", "name", "email", "segment", "suppressed"} {
		if !hasIssue(issues, p, CodeMissing) {
			t.Errorf("no missing for %s: %+v", p, issues)
		}
	}
	if len(issues) != 5 {
		t.Errorf("want 5 missing issues (notes is optional), got %+v", issues)
	}

	type Num struct {
		A int     `json:"a"`
		B uint8   `json:"b"`
		C float64 `json:"c"`
		D string  `json:"d"`
	}
	for _, body := range []string{
		`{"a":1.5,"b":1,"c":1,"d":"x"}`,
		`{"a":"1","b":1,"c":1,"d":"x"}`,
		`{"a":1e3,"b":1,"c":1,"d":"x"}`,
		`{"a":1,"b":300,"c":1,"d":"x"}`,
		`{"a":null,"b":1,"c":1,"d":"x"}`,
		`{"a":1,"b":1,"c":true,"d":"x"}`,
		`{"a":1,"b":1,"c":1,"d":3}`,
	} {
		_, err := Decode[Num]([]byte(body))
		if issues := issuesOf(t, err); len(issues) != 1 || issues[0].Code != CodeType {
			t.Errorf("%s: issues %+v", body, issues)
		}
	}
	if _, err := Decode[Num]([]byte(`{"a":-3,"b":255,"c":-1.25,"d":""}`)); err != nil {
		t.Errorf("valid numbers rejected: %v", err)
	}
}

func TestDecodeSyntaxAndRoot(t *testing.T) {
	for _, body := range []string{``, `{`, `{} {}`, `{"id":1} x`} {
		_, err := Decode[Contact]([]byte(body))
		issues := issuesOf(t, err)
		if len(issues) != 1 || issues[0].Code != CodeSyntax {
			t.Errorf("%q: %+v", body, issues)
		}
	}
	_, err := Decode[Contact]([]byte(`[]`))
	if issues := issuesOf(t, err); len(issues) != 1 || issues[0].Code != CodeType {
		t.Errorf("array root: %+v", issues)
	}
	_, err = Decode[Contact]([]byte(`null`))
	if issues := issuesOf(t, err); len(issues) != 1 || issues[0].Code != CodeType {
		t.Errorf("null root: %+v", issues)
	}
	if got := (DecodeError{}).Error(); got == "" {
		t.Errorf("empty Error()")
	}
	de := &DecodeError{Issues: []Issue{{Path: []string{"a", "b"}, Code: CodeMin, Message: "m"}}}
	if !strings.Contains(de.Error(), "a.b: m") {
		t.Errorf("Error() = %q", de.Error())
	}
	if (&DecodeError{}).StatusCode() != 400 {
		t.Errorf("status")
	}
}

type Nested struct {
	Inner struct {
		Name string `json:"name" schema:"nonempty"`
	} `json:"inner"`
	Items []Item          `json:"items" schema:"max=2"`
	Tags  map[string]Item `json:"tags"`
	When  time.Time       `json:"when"`
	Raw   json.RawMessage `json:"raw"`
	Any   any             `json:"any"`
}

type Item struct {
	N int `json:"n" schema:"min=2"`
}

func TestDecodeNestedPaths(t *testing.T) {
	body := `{"inner":{"name":""},"items":[{"n":2},{"n":1},{"n":5}],"tags":{"b":{"n":0},"a":{"n":9}},"when":"2026-10-15T10:00:00Z","raw":{"x":[1, 2]},"any":null}`
	v, err := Decode[Nested]([]byte(body))
	issues := issuesOf(t, err)
	for _, want := range []struct{ p, c string }{
		{"inner.name", CodeNonEmpty},
		{"items", CodeMax},
		{"items.1.n", CodeMin},
		{"tags.b.n", CodeMin},
	} {
		if !hasIssue(issues, want.p, want.c) {
			t.Errorf("missing %s/%s in %+v", want.p, want.c, issues)
		}
	}
	if len(issues) != 4 {
		t.Errorf("issues: %+v", issues)
	}
	_ = v

	ok := `{"inner":{"name":"x"},"items":[{"n":2}],"tags":{},"when":"2026-10-15T10:00:00+02:00","raw":{"x":[1, 2]},"any":{"k":true}}`
	v, err = Decode[Nested]([]byte(ok))
	if err != nil {
		t.Fatalf("ok body: %v", err)
	}
	if string(v.Raw) != `{"x":[1, 2]}` {
		t.Errorf("RawMessage not preserved: %q", v.Raw)
	}
	if v.When.Year() != 2026 || v.Any.(map[string]any)["k"] != true {
		t.Errorf("when/any: %v %v", v.When, v.Any)
	}
	if _, err := Decode[Nested]([]byte(`{"inner":{"name":"x"},"items":[],"tags":{},"when":"yesterday","raw":1,"any":1}`)); err != nil {
		issues := issuesOf(t, err)
		if !hasIssue(issues, "when", CodeType) {
			t.Errorf("bad time: %+v", issues)
		}
	} else {
		t.Errorf("bad time accepted")
	}
	_, err = Decode[Nested]([]byte(`{"inner":{"name":"x"},"items":{},"tags":[],"when":"2026-10-15T10:00:00Z","raw":null}`))
	issues = issuesOf(t, err)
	if !hasIssue(issues, "items", CodeType) || !hasIssue(issues, "tags", CodeType) || !hasIssue(issues, "any", CodeMissing) {
		t.Errorf("shape issues: %+v", issues)
	}
}

func TestRawNullAndInterfaceAccepted(t *testing.T) {
	type S struct {
		Raw json.RawMessage `json:"raw"`
	}
	v, err := Decode[S]([]byte(`{"raw":null}`))
	if err != nil || string(v.Raw) != "null" {
		t.Errorf("raw null: %q %v", v.Raw, err)
	}
}

type Rules struct {
	U     uint           `json:"u" schema:"min=2,max=9"`
	F     float64        `json:"f" schema:"max=1.5,min=-1"`
	S     string         `json:"s" schema:"max=3"`
	L     []int          `json:"l" schema:"nonempty,max=2"`
	M     map[string]int `json:"m" schema:"min=1"`
	P     *string        `json:"p" schema:"pattern=digits,nonempty"`
	Lines string         `json:"lines" schema:"singleline"`
}

func TestRules(t *testing.T) {
	RegisterPattern("digits", regexp.MustCompile(`^[0-9]+$`))
	ok := `{"u":5,"f":1,"s":"héé","l":[1],"m":{"a":1},"p":"123","lines":"one"}`
	if _, err := Decode[Rules]([]byte(ok)); err != nil {
		t.Fatalf("ok: %v", err)
	}
	bad := `{"u":1,"f":2,"s":"héllo","l":[],"m":{},"p":"abc","lines":"a\nb"}`
	_, err := Decode[Rules]([]byte(bad))
	issues := issuesOf(t, err)
	for _, want := range []struct{ p, c string }{
		{"u", CodeMin}, {"f", CodeMax}, {"s", CodeMax}, {"l", CodeNonEmpty},
		{"m", CodeMin}, {"p", CodePattern}, {"lines", CodeSingleLine},
	} {
		if !hasIssue(issues, want.p, want.c) {
			t.Errorf("missing %s/%s in %+v", want.p, want.c, issues)
		}
	}
	_, err = Decode[Rules]([]byte(`{"u":10,"f":-2,"s":"","l":[1,2,3],"m":{"a":1},"p":"","lines":""}`))
	issues = issuesOf(t, err)
	for _, want := range []struct{ p, c string }{
		{"u", CodeMax}, {"f", CodeMin}, {"l", CodeMax}, {"p", CodePattern},
	} {
		if !hasIssue(issues, want.p, want.c) {
			t.Errorf("missing %s/%s in %+v", want.p, want.c, issues)
		}
	}
	// nonempty on an empty string (and on a pointer to it) fails once per rule.
	_, err = Decode[Rules]([]byte(`{"u":2,"f":0,"s":"","l":[1],"m":{"a":1},"p":"","lines":""}`))
	if !hasIssue(issuesOf(t, err), "p", CodeNonEmpty) {
		t.Errorf("nonempty through pointer not checked: %v", err)
	}
	if _, err := Decode[Rules]([]byte(`{"u":2,"f":0,"s":"","l":[1],"m":{"a":1},"p":null,"lines":""}`)); err != nil {
		t.Errorf("null pointer should skip rules: %v", err)
	}
}

func TestEnumPointerAndPlainValue(t *testing.T) {
	type E struct {
		L  Level  `json:"l"`
		PL *Level `json:"pl"`
	}
	if _, err := Decode[E]([]byte(`{"l":1,"pl":2}`)); err != nil {
		t.Errorf("valid enums: %v", err)
	}
	_, err := Decode[E]([]byte(`{"l":3,"pl":-1}`))
	issues := issuesOf(t, err)
	if !hasIssue(issues, "l", CodeEnum) || !hasIssue(issues, "pl", CodeEnum) {
		t.Errorf("enum via pointer receiver: %+v", issues)
	}
	type Ls struct {
		Segs []Segment `json:"segs"`
	}
	_, err = Decode[Ls]([]byte(`{"segs":["hot","cold"]}`))
	if !hasIssue(issuesOf(t, err), "segs.1", CodeEnum) {
		t.Errorf("slice element enum: %v", err)
	}
}

func TestMapsAndSlicesSortedAndTyped(t *testing.T) {
	type K string
	type S struct {
		M map[K]int `json:"m"`
	}
	v, err := Decode[S]([]byte(`{"m":{"b":2,"a":1}}`))
	if err != nil || v.M["a"] != 1 || v.M["b"] != 2 {
		t.Fatalf("named key map: %+v %v", v, err)
	}
	_, err = Decode[S]([]byte(`{"m":{"a":"x","b":"y"}}`))
	issues := issuesOf(t, err)
	if len(issues) != 2 || issues[0].Path[1] != "a" || issues[1].Path[1] != "b" {
		t.Errorf("map issues not sorted by key: %+v", issues)
	}
}

func TestRegisterPatternPanics(t *testing.T) {
	RegisterPattern("unique-name-x", regexp.MustCompile(`x`))
	mustPanic(t, func() { RegisterPattern("unique-name-x", regexp.MustCompile(`y`)) })
	mustPanic(t, func() { RegisterPattern("", regexp.MustCompile(`y`)) })
	mustPanic(t, func() { RegisterPattern("a=b", regexp.MustCompile(`y`)) })
	mustPanic(t, func() { RegisterPattern("nilre", nil) })
}

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic")
		}
	}()
	f()
}

type badChan struct {
	C chan int `json:"c"`
}

type badFunc struct {
	F func() `json:"f"`
}

type badIface struct {
	I interface{ M() } `json:"i"`
}

type badBytes struct {
	B []byte `json:"b"`
}

type badKey struct {
	M map[int]string `json:"m"`
}

type badArray struct {
	A [2]int `json:"a"`
}

type badEmailInt struct {
	N int `json:"n" schema:"email"`
}

type badUnknownRule struct {
	S string `json:"s" schema:"regex"`
}

type badNoArg struct {
	S string `json:"s" schema:"min"`
}

type badArgNoValue struct {
	S string `json:"s" schema:"max=x"`
}

type badRepeat struct {
	S string `json:"s" schema:"trim,trim"`
}

type badPatternMissing struct {
	S string `json:"s" schema:"pattern=nowhere-registered"`
}

type badPatternArg struct {
	S string `json:"s" schema:"pattern"`
}

type badTimeRule struct {
	T time.Time `json:"t" schema:"nonempty"`
}

type badBoolRule struct {
	B bool `json:"b" schema:"max=1"`
}

type badEmbedded struct {
	Contact
}

type badUnexported struct {
	a int `schema:"min=1"`
}

type badMinFloat struct {
	F float64 `json:"f" schema:"min=nan"`
}

type badMinUint struct {
	U uint `json:"u" schema:"min=-1"`
}

type badLength struct {
	S []int `json:"s" schema:"max=-2"`
}

type badEmptyItem struct {
	S string `json:"s" schema:"trim,,max=2"`
}

type badNoArgEmail struct {
	S string `json:"s" schema:"email=1"`
}

type badSingleLineArg struct {
	S string `json:"s" schema:"singleline=1"`
}

type badMinStringNum struct {
	S string `json:"s" schema:"min=abc"`
}

type badMinIntNum struct {
	N int `json:"n" schema:"min=1.5"`
}

type badMinUintNum struct {
	N uint `json:"n" schema:"min=x"`
}

type badMinFloatNum struct {
	N float64 `json:"n" schema:"min=x"`
}

type badMinBool struct {
	N bool `json:"n" schema:"min=1"`
}

type badMaxStructTag struct {
	N Item `json:"n" schema:"max=1"`
}

type okJSONDash struct {
	Hidden int `json:"-"`
	Shown  int `json:"shown"`
	skip   int
}

type recursive struct {
	Name string      `json:"name"`
	Next *recursive  `json:"next"`
	Kids []recursive `json:"kids"`
}

func TestCompileRejects(t *testing.T) {
	cases := map[string]error{
		"chan":        compileErr[badChan](),
		"func":        compileErr[badFunc](),
		"iface":       compileErr[badIface](),
		"bytes":       compileErr[badBytes](),
		"mapkey":      compileErr[badKey](),
		"array":       compileErr[badArray](),
		"email-int":   compileErr[badEmailInt](),
		"unknown":     compileErr[badUnknownRule](),
		"noarg":       compileErr[badNoArg](),
		"argnovalue":  compileErr[badArgNoValue](),
		"repeat":      compileErr[badRepeat](),
		"pattern":     compileErr[badPatternMissing](),
		"patternarg":  compileErr[badPatternArg](),
		"timerule":    compileErr[badTimeRule](),
		"boolrule":    compileErr[badBoolRule](),
		"embedded":    compileErr[badEmbedded](),
		"unexported":  compileErr[badUnexported](),
		"minfloat":    compileErr[badMinFloat](),
		"minuint":     compileErr[badMinUint](),
		"length":      compileErr[badLength](),
		"emptyitem":   compileErr[badEmptyItem](),
		"emailarg":    compileErr[badNoArgEmail](),
		"singlearg":   compileErr[badSingleLineArg](),
		"minstring":   compileErr[badMinStringNum](),
		"minint":      compileErr[badMinIntNum](),
		"minuintnum":  compileErr[badMinUintNum](),
		"minfloatnum": compileErr[badMinFloatNum](),
		"minbool":     compileErr[badMinBool](),
		"maxstruct":   compileErr[badMaxStructTag](),
		"rootchan":    compileErr[chan int](),
	}
	dup := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeFor[int](), Tag: `json:"x"`},
		{Name: "B", Type: reflect.TypeFor[int](), Tag: `json:"x"`},
	})
	cases["dup"] = compileRuntimeType(dup)
	for name, err := range cases {
		if err == nil {
			t.Errorf("%s: Compile accepted it", name)
		}
	}
	if err := Compile[okJSONDash](); err != nil {
		t.Errorf("json:\"-\" and unexported fields: %v", err)
	}
	if err := Compile[recursive](); err != nil {
		t.Errorf("recursive type: %v", err)
	}
	if err := Compile[any](); err != nil {
		t.Errorf("any: %v", err)
	}
	if err := Compile[Contact](); err != nil {
		t.Errorf("Compile is idempotent (cached): %v", err)
	}
	if err := Compile[Contact](); err != nil {
		t.Errorf("cached second call: %v", err)
	}
	mustPanic(t, func() { _, _ = Decode[badChan]([]byte(`{}`)) })
	if _, err := Decode[recursive]([]byte(`{"name":"a","next":{"name":"b","kids":[]},"kids":[{"name":"c","kids":[]}]}`)); err != nil {
		t.Errorf("recursive decode: %v", err)
	}
}

func compileErr[T any]() error { return Compile[T]() }

// compileRuntimeType runs the same check as Compile for a type known only at run time.
func compileRuntimeType(t reflect.Type) error { return checkType(t, map[reflect.Type]bool{}) }

func TestUnregisteredPatternIsNotCached(t *testing.T) {
	type Late struct {
		S string `json:"s" schema:"pattern=late-registered"`
	}
	if err := Compile[Late](); err == nil {
		t.Fatal("unregistered pattern accepted")
	}
	RegisterPattern("late-registered", regexp.MustCompile(`^ok$`))
	if err := Compile[Late](); err != nil {
		t.Fatalf("after registration: %v", err)
	}
	if _, err := Decode[Late]([]byte(`{"s":"no"}`)); err == nil {
		t.Errorf("pattern not applied")
	}
}

func TestCompileErrorMessage(t *testing.T) {
	err := Compile[badEmailInt]()
	if err == nil || !strings.Contains(err.Error(), "does not apply") {
		t.Errorf("message: %v", err)
	}
}
