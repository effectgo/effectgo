package rtshape

import (
	"fmt"
	"strings"
	"testing"
)

func TestJSONOutputFieldSelection(t *testing.T) {
	s := describe(t, `package p
type Deep struct { Title chan int `+"`json:\"title\"`"+` }
type Base struct { Deep; ID int `+"`json:\"id\"`"+` }
type hidden struct { Visible bool `+"`json:\"visible\"`"+`; secret chan int `+"`schema:\"unknown\"`"+` }
type Named struct { Value int `+"`json:\"value\"`"+` }
type Untagged struct { Same chan int }
type Tagged struct { Winner string `+"`json:\"Same\"`"+` }
type Root struct {
 Base
 hidden
 Untagged
 Tagged
 Title string `+"`json:\"title\" schema:\"unknown\"`"+`
 Named `+"`json:\"named\"`"+`
 Invalid string `+"`json:\"bad\\\\name\"`"+`
 Ignored chan int `+"`json:\"-\"`"+`
 Dash int `+"`json:\"-,omitempty\"`"+`
}`, Options{JSONOutput: true})
	if len(s.Problems) != 0 {
		t.Fatalf("output problems: %v", messages(s))
	}
	want := map[string]Category{"id": CatInt, "visible": CatBool, "Same": CatString, "title": CatString, "named": CatStruct, "Invalid": CatString, "-": CatInt}
	if len(s.Root.Fields) != len(want) {
		t.Fatalf("fields: %v", s.Root.Fields)
	}
	for _, f := range s.Root.Fields {
		if cat, ok := want[f.JSON]; !ok || f.Type.Cat != cat {
			t.Errorf("unexpected field %q category %s", f.JSON, f.Type.Cat)
		}
	}
}

func TestJSONOutputOmission(t *testing.T) {
	s := describe(t, `package p
type Extra struct {
 Note *string `+"`json:\"note\"`"+`
 Omitted *string `+"`json:\"omitted,omitempty\"`"+`
 Values []int `+"`json:\"values\"`"+`
}
type Root struct {
 *Extra
 Object struct { Value int } `+"`json:\"object,omitempty\"`"+`
 Zero struct { Value int } `+"`json:\"zero,omitzero\"`"+`
}`, Options{JSONOutput: true})
	if len(s.Problems) != 0 {
		t.Fatal(messages(s))
	}
	for _, name := range []string{"Note", "Omitted", "Values"} {
		if !field(t, s.Root, name).OmitParent {
			t.Errorf("%s must allow omission through a nil embedded pointer", name)
		}
	}
	if !field(t, s.Root, "Note").Optional || field(t, s.Root, "Note").Omitempty || !field(t, s.Root, "Omitted").Omitempty {
		t.Fatal("promoted pointer null/omitempty changed")
	}
	if field(t, s.Root, "Object").Omitempty || !field(t, s.Root, "Zero").Omitempty {
		t.Fatal("struct omitempty and omitzero must differ")
	}
}

func TestJSONOutputAmbiguityAndInputRestrictions(t *testing.T) {
	for _, src := range []string{
		`package p; type A struct{ Value int }; type B struct{ Value int }; type Root struct{ A; B }`,
		`package p; type A struct{ Value int }; type B struct{ A }; type C struct{ A }; type Root struct{ B; C }`,
		`package p; type Root struct{ A int ` + "`json:\"value\"`" + `; B string ` + "`json:\"value\"`" + ` }`,
	} {
		s := describe(t, src, Options{JSONOutput: true})
		wantProblem(t, s, "ambiguous output json name")
		if len(s.Root.Fields) != 0 {
			t.Fatal("ambiguous fields must not be described")
		}
	}
	src := `package p; type A struct{ Value int }; type Root struct{ *A }`
	wantProblem(t, describe(t, src, Options{}), "embedded fields are not supported")
	wantProblem(t, describe(t, src, Options{JSONBody: true}), "embedded fields are not supported")
	recursive := describe(t, `package p; type Root struct{ *Root; Value int }`, Options{JSONOutput: true})
	if len(recursive.Problems) != 0 || len(recursive.Root.Fields) != 1 {
		t.Fatalf("recursive embedding changed fields: %v", messages(recursive))
	}
}

func TestJSONOutputRepeatedDiamonds(t *testing.T) {
	// Eighty diamonds have only 241 embedded types, but 2^80 leaf paths.
	var src strings.Builder
	src.WriteString("package p; type Diamond0 struct{ Value chan int };\n")
	for i := 1; i <= 80; i++ {
		fmt.Fprintf(&src, "type Left%d struct{ *Diamond%d }; type Right%d struct{ Diamond%d }; type Diamond%d struct{ Left%d; Right%d };\n", i, i-1, i, i-1, i, i, i)
	}
	for _, direct := range []bool{true, false} {
		t.Run(fmt.Sprintf("direct=%t", direct), func(t *testing.T) {
			root := "type Root struct{ Diamond80 }"
			if direct {
				root = "type Root struct{ Diamond80; Value string }"
			}
			s := describe(t, src.String()+root, Options{JSONOutput: true})
			if direct {
				if len(s.Problems) != 0 || len(s.Root.Fields) != 1 {
					t.Fatalf("direct field must dominate all diamond paths: fields=%v problems=%v", s.Root.Fields, messages(s))
				}
				f := field(t, s.Root, "Value")
				if f.Type.Cat != CatString || f.OmitParent {
					t.Fatalf("direct field inherited a shadowed leaf's shape or nil ancestor: %+v", f)
				}
			} else {
				wantProblem(t, s, "ambiguous output json name")
				if len(s.Problems) != 1 || len(s.Root.Fields) != 0 {
					t.Fatalf("repeated leaf must produce one ambiguity without describing its channel: fields=%v problems=%v", s.Root.Fields, messages(s))
				}
			}
		})
	}
}

func TestJSONOutputEmbeddingPaths(t *testing.T) {
	for _, tc := range []struct {
		name        string
		src         string
		fields      []string
		omitParents []bool
	}{
		{
			name: "declaration order",
			src: `package p
type Inner struct{ First int }
type Deep struct{ Inner }
type Root struct{ Deep; Second int }`,
			fields:      []string{"First", "Second"},
			omitParents: []bool{false, false},
		},
		{
			name: "nearest path",
			src: `package p
type Shared struct{ Value int }
type Inner struct{ Shared }
type Deep struct{ *Inner }
type Root struct{ Deep; Shared }`,
			fields:      []string{"Value"},
			omitParents: []bool{false},
		},
		{
			name: "tag beats repeated untagged paths",
			src: `package p
type Leaf struct{ Value chan int }
type Left struct{ *Leaf }
type Right struct{ Leaf }
type Tagged struct{ Winner string ` + "`json:\"Value\"`" + ` }
type Other struct{ *Tagged }
type Root struct{ Left; Right; Other }`,
			fields:      []string{"Winner"},
			omitParents: []bool{true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := describe(t, tc.src, Options{JSONOutput: true})
			if len(s.Problems) != 0 || len(s.Root.Fields) != len(tc.fields) {
				t.Fatalf("fields=%v problems=%v", s.Root.Fields, messages(s))
			}
			for i, f := range s.Root.Fields {
				if f.Name != tc.fields[i] || f.OmitParent != tc.omitParents[i] {
					t.Errorf("field %d: name=%q omitParent=%t; want name=%q omitParent=%t", i, f.Name, f.OmitParent, tc.fields[i], tc.omitParents[i])
				}
			}
		})
	}
}
