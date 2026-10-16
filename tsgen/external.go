package tsgen

import (
	"go/ast"
	"go/types"
)

// externalErrors reads direct ExternalError[E]() route options. Reject opaque
// options rather than silently generating an incomplete middleware contract.
func (st *state) externalErrors(call *ast.CallExpr, r *Route) {
	for _, arg := range call.Args[3:] {
		option, ok := arg.(*ast.CallExpr)
		if !ok || len(option.Args) != 0 {
			r.Problems = append(r.Problems, r.Pos+": route options must be direct web.ExternalError[E]() calls")
			continue
		}
		index, ok := option.Fun.(*ast.IndexExpr)
		if !ok {
			r.Problems = append(r.Problems, r.Pos+": route options must be direct web.ExternalError[E]() calls")
			continue
		}
		var obj types.Object
		switch f := index.X.(type) {
		case *ast.SelectorExpr:
			obj = st.pass.TypesInfo.Uses[f.Sel]
		case *ast.Ident:
			obj = st.pass.TypesInfo.Uses[f]
		}
		if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != webPath || obj.Name() != "ExternalError" {
			r.Problems = append(r.Problems, r.Pos+": unsupported route option; expected web.ExternalError[E]()")
			continue
		}
		named, ok := types.Unalias(st.pass.TypesInfo.TypeOf(index.Index)).(*types.Named)
		if !ok {
			r.Problems = append(r.Problems, r.Pos+": external error must be a named struct")
			continue
		}
		if _, ok := named.Underlying().(*types.Struct); !ok {
			r.Problems = append(r.Problems, r.Pos+": external error must be a named struct")
			continue
		}
		key := named.Obj().Pkg().Path() + "." + named.Obj().Name()
		st.describeNamed(named, r)
		replaced := false
		for i := range r.Errors {
			if r.Errors[i].Key == key {
				r.Errors[i].Untagged = true
				replaced = true
				break
			}
		}
		if replaced {
			continue
		}
		r.Errors = append(r.Errors, ErrRef{Key: key, Name: named.Obj().Name(), Status: st.statusOf(named, r), Untagged: true})
	}
}
