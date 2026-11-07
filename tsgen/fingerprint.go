package tsgen

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/types"
	"slices"
	"strings"

	"github.com/effectgo/effectgo/internal/rtshape"
)

// Fingerprint identifies the analyzed routes and DTO schemas, independently of
// source positions, package traversal order, or TypeScript formatting.
func Fingerprint(results []Result) string {
	var records []string
	for _, result := range results {
		for _, route := range result.Routes {
			var b strings.Builder
			enc := json.NewEncoder(&b)
			in, out, failure := route.In, route.Out, route.ProxyFailure
			route.Pos, route.Problems = "", nil
			route.In, route.Out, route.ProxyFailure = nil, nil, nil
			_ = enc.Encode(route)
			for _, shape := range []*rtshape.Shape{in, out, failure} {
				if shape == nil {
					_ = enc.Encode(nil)
					continue
				}
				_ = enc.Encode(shape.JSONOutput)
				fingerprintType(enc, shape.Root, map[*rtshape.Type]bool{})
			}
			records = append(records, b.String())
		}
		for _, decl := range result.Decls {
			var b strings.Builder
			enc := json.NewEncoder(&b)
			_ = enc.Encode([]string{decl.Key, decl.Name})
			fingerprintType(enc, decl.Type, map[*rtshape.Type]bool{})
			records = append(records, b.String())
		}
	}
	slices.Sort(records)
	records = slices.Compact(records)
	return fmt.Sprintf("%x", sha256.Sum256([]byte("effectgo-contract-v1\n"+strings.Join(records, "\n"))))
}

func fingerprintType(enc *json.Encoder, typ *rtshape.Type, seen map[*rtshape.Type]bool) {
	if typ == nil {
		_ = enc.Encode(nil)
		return
	}
	if seen[typ] {
		_ = enc.Encode([]string{"ref", typ.Key})
		return
	}
	seen[typ] = true
	defer delete(seen, typ)
	copy := *typ
	copy.Go, copy.Elem, copy.Fields, copy.Consts = nil, nil, nil, nil
	_ = enc.Encode(copy)
	if typ.Go != nil {
		_ = enc.Encode(types.TypeString(typ.Go, nil))
	}
	for _, constant := range typ.Consts {
		_ = enc.Encode([]string{constant.Name(), constant.Val().ExactString()})
	}
	_ = enc.Encode(len(typ.Consts))
	fingerprintType(enc, typ.Elem, seen)
	for _, field := range typ.Fields {
		copy := *field
		copy.Var, copy.Type = nil, nil
		_ = enc.Encode(copy)
		fingerprintType(enc, field.Type, seen)
	}
	_ = enc.Encode(len(typ.Fields))
}
