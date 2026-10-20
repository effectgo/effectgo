package tsgen

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/faintghost/effectgo/internal/rtshape"
)

// The outer unknown codec admits the unchanged inferred input type to the
// transform; the original schema then validates the normalized value on encode.
const jsonRequestBoundary = `function omitUndefined<T>(value: T): T {
  if (Array.isArray(value)) {
    let copy: unknown[] | undefined
    for (let i = 0; i < value.length; i++) {
      const item = omitUndefined(value[i])
      if (item !== value[i]) {
        copy ??= value.slice()
        copy[i] = item
      }
    }
    return (copy ?? value) as T
  }
  if (value === null || typeof value !== "object") return value
  const prototype = Object.getPrototypeOf(value)
  if (prototype !== Object.prototype && prototype !== null) return value
  const record = value as Record<string, unknown>
  let copy: Record<string, unknown> | undefined
  for (const key of Object.keys(record)) {
    const item = omitUndefined(record[key])
    if (item === undefined || item !== record[key]) {
      const target: Record<string, unknown> = copy ??= Object.setPrototypeOf({ ...record }, prototype)
      if (item === undefined) delete target[key]
      else Object.defineProperty(target, key, { value: item, enumerable: true, configurable: true, writable: true })
    }
  }
  return (copy ?? value) as T
}

function jsonRequest<S extends Schema.Constraint>(schema: S) {
  // Normalize before the original schema encodes; its checks and inferred input type remain authoritative.
  return Schema.decodeTo<Schema.Codec<S["Type"]>, S>(Schema.Unknown as Schema.Codec<S["Type"]>, {
    decode: SchemaGetter.passthrough(),
    encode: SchemaGetter.transform((value: S["Type"]) => omitUndefined(value)),
  })(schema)
}

`

// jsonPayload follows encoding/json's optional fields and unknown object keys.
// Recursive objects get local, typed suspended schemas, keeping their names out
// of the module's public declaration namespace and out of strict request schemas.
func (e *emitter) jsonPayload(root *rtshape.Type, fields []*rtshape.Field) string {
	if schema, _, custom := wire(root, true); custom {
		return schema
	}
	g := jsonBodySchema{emitter: e}
	states := map[*rtshape.Type]uint8{}
	var objects []*rtshape.Type
	recursive := false
	var visit func(*rtshape.Type)
	visit = func(t *rtshape.Type) {
		if t == nil {
			return
		}
		if _, _, custom := wire(t, true); custom {
			return
		}
		if states[t] != 0 {
			recursive = recursive || states[t] == 1
			return
		}
		states[t] = 1
		if t.Cat == rtshape.CatStruct {
			objects = append(objects, t)
		}
		for _, field := range t.Fields {
			visit(field.Type)
		}
		visit(t.Elem)
		states[t] = 2
	}
	for _, field := range fields {
		visit(field.Type)
	}
	if !recursive {
		return "Schema.NullOr(" + g.object(fields) + ")"
	}
	g.names = make(map[*rtshape.Type]string, len(objects))
	for i, object := range objects {
		g.names[object] = fmt.Sprintf("JsonBody%d", i)
	}
	var b strings.Builder
	b.WriteString("(() => { ")
	for _, object := range objects {
		name := g.names[object]
		fmt.Fprintf(&b, "type %s = %s; ", name, g.objectType(object.Fields))
	}
	for _, object := range objects {
		name := g.names[object]
		fmt.Fprintf(&b, "const schema%s: Schema.Codec<%s, %s> = Schema.suspend(() => %s); ", name, name, name, g.object(object.Fields))
	}
	b.WriteString("return Schema.NullOr(" + g.object(fields) + "); })()")
	return b.String()
}

type jsonBodySchema struct {
	emitter *emitter
	names   map[*rtshape.Type]string
}

func (g *jsonBodySchema) object(fields []*rtshape.Field) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		schema := g.expr(f.Type)
		if quotedScalar(f) {
			// strconv's accepted quoted scalar spellings differ from JSON.parse.
			// Preserve the string wire type; the server validates its contents.
			schema = "Schema.NullOr(Schema.String)"
		}
		parts = append(parts, key(f.JSON)+": Schema.optionalKey("+schema+")")
	}
	return "Schema.StructWithRest(Schema.Struct({ " + strings.Join(parts, ", ") + " }), [Schema.Record(Schema.String, Schema.Unknown)])"
}

func (g *jsonBodySchema) expr(t *rtshape.Type) string {
	if schema, _, custom := wire(t, true); custom {
		return schema
	}
	var schema string
	switch t.Cat {
	case rtshape.CatStruct:
		if name := g.names[t]; name != "" {
			schema = "schema" + name
		} else {
			schema = g.object(t.Fields)
		}
	case rtshape.CatPointer:
		return g.expr(t.Elem)
	case rtshape.CatSlice:
		schema = "Schema.Array(" + g.expr(t.Elem) + ")"
	case rtshape.CatMap:
		schema = "Schema.Record(Schema.String, " + g.expr(t.Elem) + ")"
	case rtshape.CatEnum:
		basic := *t
		info := t.Go.Underlying().(*types.Basic).Info()
		switch {
		case info&types.IsString != 0:
			basic.Cat = rtshape.CatString
		case info&types.IsBoolean != 0:
			basic.Cat = rtshape.CatBool
		default:
			basic.Cat = rtshape.CatInt
		}
		schema = g.emitter.expr(&basic, true)
	default:
		schema = g.emitter.expr(t, true)
	}
	return "Schema.NullOr(" + schema + ")"
}

func quotedScalar(f *rtshape.Field) bool {
	if !f.JSONString {
		return false
	}
	t := f.Type
	for t.Cat == rtshape.CatPointer {
		t = t.Elem
	}
	if _, _, custom := wire(t, true); custom {
		return false
	}
	basic, ok := t.Go.Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsString|types.IsBoolean|types.IsInteger|types.IsFloat) != 0
}

func (g *jsonBodySchema) objectType(fields []*rtshape.Field) string {
	parts := make([]string, 0, len(fields)+1)
	for _, f := range fields {
		typ := g.typeExpr(f.Type)
		if quotedScalar(f) {
			typ = "string | null"
		}
		parts = append(parts, "readonly "+key(f.JSON)+"?: "+typ)
	}
	parts = append(parts, "readonly [key: string]: unknown")
	return "{ " + strings.Join(parts, "; ") + " }"
}

func (g *jsonBodySchema) typeExpr(t *rtshape.Type) string {
	if _, _, custom := wire(t, true); custom {
		return "unknown"
	}
	switch t.Cat {
	case rtshape.CatStruct:
		return g.names[t] + " | null"
	case rtshape.CatPointer:
		return g.typeExpr(t.Elem)
	case rtshape.CatSlice:
		return "ReadonlyArray<" + g.typeExpr(t.Elem) + "> | null"
	case rtshape.CatMap:
		return "{ readonly [key: string]: " + g.typeExpr(t.Elem) + " } | null"
	case rtshape.CatString, rtshape.CatTime:
		return "string | null"
	case rtshape.CatInt, rtshape.CatFloat:
		return "number | null"
	case rtshape.CatBool:
		return "boolean | null"
	case rtshape.CatEnum:
		info := t.Go.Underlying().(*types.Basic).Info()
		if info&types.IsString != 0 {
			return "string | null"
		}
		if info&types.IsBoolean != 0 {
			return "boolean | null"
		}
		return "number | null"
	default:
		return "unknown"
	}
}
