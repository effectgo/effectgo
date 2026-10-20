package rtshape

import (
	"fmt"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

// outputFields selects encoding/json's visible fields before describing their
// types: an ignored or dominated field must not introduce schemas or problems.
func (b *builder) outputFields(out *Type, t types.Type, st *types.Struct) {
	type candidate struct {
		field      *types.Var
		tag        reflect.StructTag
		name       string
		index      []int
		depth      int
		count      int
		tagged     bool
		omitParent bool
	}
	type embedded struct {
		st         *types.Struct
		index      []int
		count      int
		omitParent bool
	}
	fields := make([]candidate, 0, st.NumFields())
	queue := []embedded{{st: st, count: 1}}
	byType := map[types.Type]int{t: 0}
	// Breadth first visits each embedded type only at its nearest depth. All
	// equally short paths arrive before that type is scanned; counts saturate
	// at two because only uniqueness matters. This bounds repeated diamonds
	// by their types and fields rather than their exponentially many paths.
	// The queue grows during the loop, so its bound must be checked each time.
	for head := 0; head < len(queue); head++ {
		parent := queue[head]
		for i := range parent.st.NumFields() {
			f := parent.st.Field(i)
			ft := types.Unalias(f.Type())
			p, pointer := ft.(*types.Pointer)
			if pointer {
				ft = types.Unalias(p.Elem())
			}
			inner, isStruct := ft.Underlying().(*types.Struct)
			if !f.Exported() && (!f.Embedded() || !isStruct) {
				continue
			}
			tag := reflect.StructTag(parent.st.Tag(i))
			if tag.Get("json") == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag.Get("json"), ",")
			if !validJSONName(name) {
				name = ""
			}
			promoted := f.Embedded() && isStruct && name == ""
			if promoted {
				if j, seen := byType[ft]; seen {
					if len(queue[j].index) == len(parent.index)+1 {
						queue[j].count = min(2, queue[j].count+parent.count)
						queue[j].omitParent = queue[j].omitParent || parent.omitParent || pointer
					}
					continue
				}
			}
			index := make([]int, len(parent.index)+1)
			copy(index, parent.index)
			index[len(parent.index)] = i
			if promoted {
				byType[ft] = len(queue)
				queue = append(queue, embedded{inner, index, parent.count, parent.omitParent || pointer})
				continue
			}
			tagged := name != ""
			if name == "" {
				name = f.Name()
			}
			fields = append(fields, candidate{f, tag, name, index, len(parent.index), parent.count, tagged, parent.omitParent})
		}
	}
	// The shortest representative path also preserves declaration order for
	// fields and diagnostics, including fields promoted through deep embeds.
	slices.SortFunc(fields, func(a, b candidate) int { return slices.Compare(a.index, b.index) })
	type selection struct{ best, count int }
	byName := make(map[string]selection, len(fields))
	for i, f := range fields {
		selected, ok := byName[f.name]
		if !ok || f.depth < fields[selected.best].depth || f.depth == fields[selected.best].depth && f.tagged && !fields[selected.best].tagged {
			byName[f.name] = selection{i, f.count}
		} else if f.depth == fields[selected.best].depth && f.tagged == fields[selected.best].tagged {
			selected.count = min(2, selected.count+f.count)
			byName[f.name] = selected
		}
	}
	for i, f := range fields {
		selected := byName[f.name]
		best, count := selected.best, selected.count
		if best != i {
			continue
		}
		where := "field " + out.Name + "." + f.field.Name()
		if out.Name == "" {
			where = "field struct." + f.field.Name()
		}
		if count != 1 {
			b.problem(f.field, where, fmt.Sprintf("ambiguous output json name %q; encoding/json omits all equally dominant fields; rename or tag a field", f.name))
			continue
		}
		_, opts, _ := strings.Cut(f.tag.Get("json"), ",")
		options := strings.Split(opts, ",")
		fld := &Field{
			Var:        f.field,
			Name:       f.field.Name(),
			JSON:       f.name,
			Omitempty:  slices.Contains(options, "omitempty") && emptyJSONValue(f.field.Type()) || slices.Contains(options, "omitzero"),
			OmitParent: f.omitParent,
			JSONString: slices.Contains(options, "string"),
		}
		ft := types.Unalias(f.field.Type())
		if p, ok := ft.(*types.Pointer); ok {
			fld.Optional = true
			ft = p.Elem()
		}
		fld.Type = b.typ(ft, f.field, where)
		out.Fields = append(out.Fields, fld)
	}
}

// validJSONName mirrors encoding/json's tag name grammar. Invalid names fall
// back to the Go field name (or promotion for an embedded struct).
func validJSONName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c) && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

func emptyJSONValue(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Array:
		return u.Len() == 0
	case *types.Struct:
		return false
	default:
		return true
	}
}
