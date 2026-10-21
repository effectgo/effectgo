// Package rtshape describes a Go type the way rt/web and rt/schema read it: its
// fields with their JSON names, optionality, omitempty, path and query tags and
// parsed schema rules, the constants of an enum, and every struct and enum
// reached from it, each once.
//
// It is the one place that knows the rt rules for a type. schematag turns its
// Problems into diagnostics; the TypeScript generator (tsgen) reads the same
// description to emit schemas. A problem is reported, not fixed: the Type still
// describes what was found, with Cat set to CatUnsupported where rt would
// refuse the type.
//
// The vocabulary (vocab.go) is copied from rt/schema. schematag's test reads
// rt/schema/schema.go and fails when the copy and the source differ.
package rtshape

import (
	"crypto/sha256"
	"fmt"
	"go/types"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Category is the rt view of a type: what its JSON value is.
type Category string

const (
	CatString      Category = "string"
	CatInt         Category = "int" // every Go integer kind, signed or not
	CatFloat       Category = "float"
	CatBool        Category = "bool"
	CatTime        Category = "time"   // time.Time: an RFC 3339 string
	CatRaw         Category = "raw"    // json.RawMessage
	CatAny         Category = "any"    // empty interface
	CatSlice       Category = "slice"  // Elem is the element
	CatMap         Category = "map"    // Elem is the value; the key is a string kind
	CatStruct      Category = "struct" // Fields
	CatEnum        Category = "enum"   // named basic type with Valid() bool; Consts
	CatPointer     Category = "pointer"
	CatUnsupported Category = "unsupported" // rt refuses the type; see the Problems
	CatUnknown     Category = "unknown"     // a type parameter or an invalid type: not described
)

// Type is one type of the tree. Structs and enums are shared: a type reached
// twice is the same *Type, so a recursive type is a cycle through pointers.
type Type struct {
	Go   types.Type
	Cat  Category
	Name string // schema identifier; instantiated named types include a stable digest
	Pkg  string // the import path of a named type's package, "" otherwise
	Key  string // fully qualified Go type identity, including instantiated arguments
	// RuleCat is the category that schema rules see (rt/schema categoryOf on the
	// type without its pointers): "string", "int", "uint", "float", "slice",
	// "map", or "" when no rule applies to the kind.
	RuleCat string
	Elem    *Type          // pointer, slice and map value
	Fields  []*Field       // struct: the fields rt reads, in declaration order
	Consts  []*types.Const // enum: the constants of the type, in source order
	// MarshalJSON, UnmarshalJSON and MarshalText report that a named type has
	// the method, on its value or its pointer. A type with its own JSON methods
	// has the JSON form they write or read, which the Go fields do not state.
	MarshalJSON   bool
	UnmarshalJSON bool
	MarshalText   bool
}

// Field is one field rt reads.
type Field struct {
	Var        *types.Var
	Name       string // the Go name
	JSON       string // the json name; the Go name when the tag gives none
	Omitempty  bool   // json:",omitempty"
	OmitParent bool   // a nil embedded pointer can omit this promoted field
	JSONString bool   // encoding/json's ,string option for scalar fields
	Path       string // the path tag, "" when there is none
	Query      string // the query tag, "" when there is none
	Header     string // the header tag, "" when there is none
	Cookie     string // the cookie tag, "" when there is none
	Form       string // multipart field name
	File       bool   // multipart web.File rather than a scalar field
	Rules      []Rule // the schema rules that parsed and fit the field
	Optional   bool   // the field is declared with a pointer type
	Type       *Type  // the field's type; with Optional, the type the pointer points to
}

// Rule is one parsed schema rule, such as max=200.
type Rule struct {
	Name   string
	Arg    string
	HasArg bool
}

// Problem is something rt would refuse. Owner is the field that holds the type
// (nil for the root); Where names the field ("field T.F") or is "" for the root.
// Msg is the explanation, without the location: the caller adds it.
type Problem struct {
	Owner *types.Var
	Where string
	Msg   string
}

// Shape is the description of one root type.
type Shape struct {
	Root       *Type
	Types      []*Type   // every struct and enum reached, once each, in first-seen order
	Problems   []Problem // in the order the walk found them
	JSONOutput bool      // encoding/json output rather than schema input
}

// Options configure Describe.
type Options struct {
	// Multipart includes file inputs tagged form:"name" json:"-". JSON-only
	// callers leave it false so ignored fields retain their existing meaning.
	Multipart bool
	// JSONBody describes encoding/json input rather than schema validation.
	// Byte slices are opaque JSON values; schema validation still rejects them.
	JSONBody bool
	// JSONOutput follows encoding/json field promotion and dominance. Schema
	// input keeps its stricter field and tag rules.
	JSONOutput bool
	// Qualifier names the types in messages. Nil qualifies by full path.
	Qualifier types.Qualifier
	// Sealed is called for every named type the walk visits. It returns the
	// message of a diagnostic about that type, or "" when there is none; the
	// message becomes a Problem of the field that holds it. schematag uses it
	// for the sealed-enum-without-Valid diagnostic, which is its own rule.
	Sealed func(n *types.Named) string
}

// Describe describes t and everything reachable from it.
func Describe(t types.Type, opt Options) *Shape {
	t = types.Unalias(t)
	b := &builder{opt: opt, root: t, shape: &Shape{JSONOutput: opt.JSONOutput}, memo: map[string]*Type{}}
	b.shape.Root = b.typ(t, nil, "")
	return b.shape
}

// HasValid reports whether n has a method Valid() bool, on the value or the
// pointer: the two method sets schema.Decode checks.
func HasValid(n *types.Named) bool {
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(n), true, n.Obj().Pkg(), "Valid")
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Params().Len() == 0 && sig.Results().Len() == 1 &&
		types.Identical(sig.Results().At(0).Type(), types.Typ[types.Bool])
}

// hasMethod reports whether n, or *n, has the method name with the given number
// of parameters and results. The signature is not checked beyond its counts: a
// method of another shape is not the encoding interface's method.
func hasMethod(n *types.Named, name string, params, results int) bool {
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(n), true, n.Obj().Pkg(), name)
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Params().Len() == params && sig.Results().Len() == results
}

// BasicKind is the reflect kind of a basic type (reflect.Invalid for the kinds
// that reflect has no name for here: untyped values).
func BasicKind(b *types.Basic) reflect.Kind {
	switch b.Kind() {
	case types.Bool:
		return reflect.Bool
	case types.String:
		return reflect.String
	case types.Int:
		return reflect.Int
	case types.Int8:
		return reflect.Int8
	case types.Int16:
		return reflect.Int16
	case types.Int32:
		return reflect.Int32
	case types.Int64:
		return reflect.Int64
	case types.Uint:
		return reflect.Uint
	case types.Uint8:
		return reflect.Uint8
	case types.Uint16:
		return reflect.Uint16
	case types.Uint32:
		return reflect.Uint32
	case types.Uint64:
		return reflect.Uint64
	case types.Uintptr:
		return reflect.Uintptr
	case types.Float32:
		return reflect.Float32
	case types.Float64:
		return reflect.Float64
	case types.Complex64:
		return reflect.Complex64
	case types.Complex128:
		return reflect.Complex128
	case types.UnsafePointer:
		return reflect.UnsafePointer
	}
	return reflect.Invalid
}

// builder holds the state of one Describe call.
type builder struct {
	opt   Options
	root  types.Type
	shape *Shape
	memo  map[string]*Type // struct and enum types, by TypeString without a qualifier
}

func (b *builder) problem(owner *types.Var, where, msg string) {
	b.shape.Problems = append(b.shape.Problems, Problem{Owner: owner, Where: where, Msg: msg})
}

func (b *builder) typeString(t types.Type) string {
	if b.opt.Qualifier == nil {
		return types.TypeString(t, nil)
	}
	return types.TypeString(t, b.opt.Qualifier)
}

// typ describes a type and everything reachable from it. owner and where are the
// field that holds the type, for the problems found here.
func (b *builder) typ(t types.Type, owner *types.Var, where string) *Type {
	t = types.Unalias(t)
	if isNamed(t, "time", "Time") {
		return &Type{Go: t, Cat: CatTime, RuleCat: ""}
	}
	if isNamed(t, "encoding/json", "RawMessage") {
		return &Type{Go: t, Cat: CatRaw, RuleCat: ""}
	}
	if _, ok := t.(*types.TypeParam); ok {
		return &Type{Go: t, Cat: CatUnknown}
	}
	out := &Type{Go: t, RuleCat: ruleCategory(t)}
	if n, ok := t.(*types.Named); ok {
		out.Name = n.Obj().Name()
		if p := n.Obj().Pkg(); p != nil {
			out.Pkg = p.Path()
		}
		out.Key = types.TypeString(n, nil)
		if n.TypeArgs().Len() > 0 {
			out.Name += fmt.Sprintf("_%x", sha256.Sum256([]byte(out.Key)))
		}
		out.MarshalJSON = hasMethod(n, "MarshalJSON", 0, 2)
		out.UnmarshalJSON = hasMethod(n, "UnmarshalJSON", 1, 1)
		out.MarshalText = hasMethod(n, "MarshalText", 0, 2)
		if b.opt.Sealed != nil {
			if msg := b.opt.Sealed(n); msg != "" {
				b.problem(owner, where, msg)
			}
		}
	}
	if b.opt.JSONOutput && (out.MarshalJSON || out.MarshalText) {
		out.Cat = CatAny // the encoding method owns the wire shape
		if _, ok := t.Underlying().(*types.Struct); ok {
			out.Cat = CatStruct
			b.shape.Types = append(b.shape.Types, out)
		}
		return out
	}
	if b.opt.JSONBody && out.UnmarshalJSON && t != b.root {
		out.Cat = CatAny // the decoder, rather than exported fields, determines the wire type
		return out
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		if u.Kind() == types.Invalid {
			out.Cat = CatUnknown
			return out
		}
		if !supportedBasic(u) {
			out.Cat = CatUnsupported
			b.problem(owner, where, fmt.Sprintf("type %s is not supported by schema; use a string, a number, a bool, time.Time, a struct, a slice, a map with string keys, or a pointer to one of them", b.typeString(t)))
			return out
		}
		out.Cat = basicCategory(u)
		if n, ok := t.(*types.Named); ok && HasValid(n) {
			out.Cat = CatEnum
			out.Consts = enumConsts(n)
		}
		return out
	case *types.Pointer:
		out.Cat = CatPointer
		out.Elem = b.typ(u.Elem(), owner, where)
		return out
	case *types.Slice:
		if eb, ok := u.Elem().Underlying().(*types.Basic); ok && eb.Kind() == types.Uint8 {
			if b.opt.JSONBody {
				out.Cat = CatAny // encoding/json accepts base64 strings and byte arrays
				return out
			}
			out.Cat = CatUnsupported
			b.problem(owner, where, "byte slices are not supported; use a string or json.RawMessage")
			return out
		}
		out.Cat = CatSlice
		out.Elem = b.typ(u.Elem(), owner, where)
		return out
	case *types.Map:
		if kb, ok := u.Key().Underlying().(*types.Basic); !ok || kb.Info()&types.IsString == 0 {
			b.problem(owner, where, fmt.Sprintf("map key must be a string kind, got %s; use map[string]T", b.typeString(u.Key())))
		}
		out.Cat = CatMap
		out.Elem = b.typ(u.Elem(), owner, where)
		return out
	case *types.Struct:
		key := types.TypeString(t, nil)
		if seen, ok := b.memo[key]; ok {
			return seen
		}
		out.Cat = CatStruct
		b.memo[key] = out
		b.shape.Types = append(b.shape.Types, out)
		if b.opt.JSONOutput {
			b.outputFields(out, t, u)
		} else {
			b.structFields(out, t, u)
		}
		return out
	case *types.Interface:
		if !u.Empty() {
			out.Cat = CatUnsupported
			b.problem(owner, where, fmt.Sprintf("non-empty interface %s is not supported; use a concrete type or any", b.typeString(t)))
			return out
		}
		out.Cat = CatAny
		return out
	}
	out.Cat = CatUnsupported
	b.problem(owner, where, fmt.Sprintf("type %s is not supported by schema; use a string, a number, a bool, time.Time, a struct, a slice, a map with string keys, or a pointer to one of them", b.typeString(t)))
	return out
}

// structFields reads the fields rt reads, as planFor does: a field with json "-"
// is skipped, an unexported one with a schema tag is a problem, an embedded one
// is a problem.
func (b *builder) structFields(out *Type, t types.Type, st *types.Struct) {
	name := "struct"
	if n, ok := t.(*types.Named); ok {
		name = n.Obj().Name()
	}
	jsonNames := map[string]bool{}
	for i := range st.NumFields() {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i))
		if b.opt.JSONBody && out.UnmarshalJSON && tag.Get("path") == "" && tag.Get("query") == "" && tag.Get("header") == "" && tag.Get("cookie") == "" {
			continue // the root custom decoder consumes its own opaque body
		}
		if form, ok := tag.Lookup("form"); b.opt.Multipart && t == b.root && ok && f.Exported() {
			where := "field " + name + "." + f.Name()
			ft := types.Unalias(f.Type())
			optional := false
			if p, ok := ft.(*types.Pointer); ok {
				optional, ft = true, types.Unalias(p.Elem())
			}
			file := isNamed(ft, "github.com/effectgo/effectgo/rt/web", "File")
			jsonName, _, _ := strings.Cut(tag.Get("json"), ",")
			if form == "" || file && tag.Get("json") != "-" || !file && jsonName != form {
				b.problem(f, where, `multipart fields require a nonempty form tag and a matching json name; files require json:"-" and web.File or *web.File`)
				continue
			}
			_, hasSchema := tag.Lookup("schema")
			_, hasPath := tag.Lookup("path")
			_, hasQuery := tag.Lookup("query")
			_, hasHeader := tag.Lookup("header")
			_, hasCookie := tag.Lookup("cookie")
			if f.Embedded() || file && hasSchema || hasPath || hasQuery || hasHeader || hasCookie {
				b.problem(f, where, "multipart file cannot be embedded or have schema or other input location tags")
				continue
			}
			var typ *Type
			if file {
				typ = &Type{Go: ft, Cat: CatAny}
			} else {
				base := ft
				if slice, ok := ft.Underlying().(*types.Slice); ok && !optional {
					base = slice.Elem()
				}
				basic, ok := base.Underlying().(*types.Basic)
				if !ok || !supportedBasic(basic) {
					b.problem(f, where, "multipart value requires a scalar, a pointer to a scalar, or a slice of scalars")
					continue
				}
				if slice, ok := ft.Underlying().(*types.Slice); ok {
					if basic, ok := slice.Elem().Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
						b.problem(f, where, "byte slices are not supported; use string or json.RawMessage")
						continue
					}
					typ = &Type{Go: ft, Cat: CatSlice, Elem: b.typ(slice.Elem(), f, where), RuleCat: "slice"}
				} else {
					typ = b.typ(ft, f, where)
				}
			}
			fld := &Field{Var: f, Name: f.Name(), JSON: form, Form: form, File: file, Optional: optional, Type: typ}
			if !file {
				b.rules(tag.Get("schema"), f, where, fld)
			}
			out.Fields = append(out.Fields, fld)
			continue
		}
		if tag.Get("json") == "-" {
			continue
		}
		schemaTag := tag.Get("schema")
		if !f.Exported() {
			if schemaTag != "" {
				b.problem(f, "field "+name+"."+f.Name(), `unexported field has a schema tag; schema reads only exported fields, so export it or remove the tag`)
			}
			continue
		}
		where := "field " + name + "." + f.Name()
		if f.Embedded() {
			b.problem(f, where, "embedded fields are not supported; declare the field by name")
			continue
		}
		jsonName, opts, _ := strings.Cut(tag.Get("json"), ",")
		if jsonName == "" {
			jsonName = f.Name()
		}
		if jsonNames[jsonName] {
			b.problem(f, where, fmt.Sprintf("json name %q is used by two fields of %s; rename one", jsonName, name))
		}
		jsonNames[jsonName] = true
		fld := &Field{
			Var:        f,
			Name:       f.Name(),
			JSON:       jsonName,
			Omitempty:  slices.Contains(strings.Split(opts, ","), "omitempty"),
			JSONString: slices.Contains(strings.Split(opts, ","), "string"),
			Path:       tag.Get("path"),
			Query:      tag.Get("query"),
			Header:     tag.Get("header"),
			Cookie:     tag.Get("cookie"),
		}
		if !b.opt.JSONBody || t == b.root && (fld.Path != "" || fld.Query != "" || fld.Header != "" || fld.Cookie != "") {
			b.rules(schemaTag, f, where, fld)
		}
		if p, ok := f.Type().(*types.Pointer); ok {
			fld.Optional = true
			fld.Type = b.typ(p.Elem(), f, where)
		} else {
			fld.Type = b.typ(f.Type(), f, where)
		}
		out.Fields = append(out.Fields, fld)
	}
}

// rules parses the schema tag of one field, as parseRules does. A rule that does
// not parse or does not fit the field is a problem and is not kept in fld.Rules.
func (b *builder) rules(tag string, f *types.Var, where string, fld *Field) {
	if tag == "" {
		return
	}
	base := f.Type()
	for {
		p, ok := base.Underlying().(*types.Pointer)
		if !ok {
			break
		}
		base = p.Elem()
	}
	cat := ruleCategory(base)
	seen := map[string]bool{}
	for _, item := range strings.Split(tag, ",") {
		item = strings.TrimSpace(item)
		name, arg, hasArg := strings.Cut(item, "=")
		prefix := fmt.Sprintf("schema tag %q: ", tag)
		if name == "" {
			b.problem(f, where, prefix+"empty rule; remove the extra comma")
			continue
		}
		if seen[name] {
			b.problem(f, where, prefix+fmt.Sprintf("rule %q is repeated; keep one", name))
			continue
		}
		seen[name] = true
		if msg := b.ruleError(name, arg, hasArg, cat, base); msg != "" {
			b.problem(f, where, prefix+msg)
			continue
		}
		fld.Rules = append(fld.Rules, Rule{Name: name, Arg: arg, HasArg: hasArg})
	}
}

// ruleError returns why rule name (with its argument) does not fit a field of
// rule category cat and base type base, or "" when it does (parseRule).
func (b *builder) ruleError(name, arg string, hasArg bool, cat string, base types.Type) string {
	spec, known := Rules[name]
	if !known {
		return fmt.Sprintf("unknown rule %q; the rules are %s", name, RuleNames())
	}
	if spec.NoArg && hasArg {
		return fmt.Sprintf("rule %q takes no argument; write %s", name, name)
	}
	if !spec.NoArg && !hasArg {
		if name == "min" || name == "max" {
			return fmt.Sprintf("rule %q needs a value; write %s=N", name, name)
		}
		return fmt.Sprintf("rule %q needs a pattern name; write pattern=NAME, a name passed to schema.RegisterPattern", name)
	}
	if !slices.Contains(spec.Cats, cat) {
		fix := "it needs a string field: change the field type to string, or remove the rule"
		if name == "nonempty" {
			fix = "it applies to string, slice and map fields"
		}
		if name == "min" || name == "max" {
			fix = "it applies to numbers, strings, slices and maps"
		}
		return fmt.Sprintf("rule %q does not apply to %s; %s", name, b.typeString(base), fix)
	}
	switch name {
	case "pattern":
		if arg == "" {
			return `rule "pattern" needs a pattern name; write pattern=NAME`
		}
	case "min", "max":
		switch cat {
		case "int":
			if _, err := strconv.ParseInt(arg, 10, 64); err != nil {
				return fmt.Sprintf("rule %q: %q is not an integer; write %s=N with a whole number", name, arg, name)
			}
		case "uint":
			if _, err := strconv.ParseUint(arg, 10, 64); err != nil {
				return fmt.Sprintf("rule %q: %q is not an unsigned integer; write %s=N with a whole number of at least 0", name, arg, name)
			}
		case "float":
			if v, err := strconv.ParseFloat(arg, 64); err != nil || v != v {
				return fmt.Sprintf("rule %q: %q is not a number; write %s=N", name, arg, name)
			}
		default: // string, slice, map: a length
			if v, err := strconv.ParseInt(arg, 10, 64); err != nil || v < 0 {
				return fmt.Sprintf("rule %q: %q is not a length; write %s=N with a whole number of at least 0", name, arg, name)
			}
		}
	}
	return ""
}

// RuleNames lists the rules of the vocabulary, sorted, comma-separated.
func RuleNames() string {
	var names []string
	for n := range Rules {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ruleCategory is the category schema rules see for t (rt/schema categoryOf).
func ruleCategory(t types.Type) string {
	if isNamed(t, "time", "Time") || isNamed(t, "encoding/json", "RawMessage") {
		return ""
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return KindCategory[BasicKind(u)]
	case *types.Slice:
		return "slice"
	case *types.Map:
		return "map"
	}
	return ""
}

// basicCategory is the Category of a supported basic type.
func basicCategory(b *types.Basic) Category {
	switch BasicKind(b) {
	case reflect.String:
		return CatString
	case reflect.Bool:
		return CatBool
	case reflect.Float32, reflect.Float64:
		return CatFloat
	}
	return CatInt
}

// supportedBasic reports whether schema decodes a basic type.
func supportedBasic(b *types.Basic) bool {
	return slices.Contains(SupportedKinds, BasicKind(b))
}

// enumConsts lists the constants of type n declared in its package, in source
// order.
func enumConsts(n *types.Named) []*types.Const {
	p := n.Obj().Pkg()
	if p == nil {
		return nil
	}
	scope := p.Scope()
	var out []*types.Const
	for _, name := range scope.Names() {
		if c, ok := scope.Lookup(name).(*types.Const); ok && types.Identical(c.Type(), n) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pos() < out[j].Pos() })
	return out
}

// isNamed reports whether t is the named type pkgPath.name.
func isNamed(t types.Type, pkgPath, name string) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == pkgPath && n.Obj().Name() == name
}
