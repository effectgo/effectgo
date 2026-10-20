package tsgen

import (
	"go/ast"
	"go/constant"
	"go/types"
	"slices"

	"github.com/faintghost/effectgo/internal/rtshape"
)

const routeOptionsHint = "expected direct web.ExternalError[E](), web.SuccessStatus(code), web.SuccessStatuses(codes...), web.SuccessStatusRange(first, last), web.JSONBody(), web.JSONBodyExact(), web.JSONBodyEmpty(), web.JSONBodyBestEffort(), or web.FormBody() calls"

// routeOptions rejects opaque options instead of inventing an HTTP contract.
func (st *state) routeOptions(call *ast.CallExpr, r *Route) {
	seenStatus, seenStatuses, seenJSON := false, false, false
	for _, arg := range call.Args[3:] {
		option, ok := arg.(*ast.CallExpr)
		if !ok {
			r.Problems = append(r.Problems, r.Pos+": route options must be direct calls; "+routeOptionsHint)
			continue
		}
		fun := option.Fun
		index, generic := fun.(*ast.IndexExpr)
		if generic {
			fun = index.X
		}
		var obj types.Object
		switch f := fun.(type) {
		case *ast.SelectorExpr:
			obj = st.pass.TypesInfo.Uses[f.Sel]
		case *ast.Ident:
			obj = st.pass.TypesInfo.Uses[f]
		}
		if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != webPath {
			r.Problems = append(r.Problems, r.Pos+": unsupported route option; "+routeOptionsHint)
			continue
		}
		switch obj.Name() {
		case "FormBody":
			if generic || len(option.Args) != 0 {
				r.Problems = append(r.Problems, r.Pos+": FormBody requires a direct call without arguments")
			}
		case "JSONBody", "JSONBodyExact", "JSONBodyEmpty", "JSONBodyBestEffort":
			if generic || len(option.Args) != 0 || seenJSON {
				r.Problems = append(r.Problems, r.Pos+": JSON body mode must be declared once with a direct call")
				continue
			}
			seenJSON, r.JSONBody = true, true
			r.JSONBodyEmpty = obj.Name() == "JSONBodyEmpty" || obj.Name() == "JSONBodyBestEffort"
		case "SuccessStatusRange":
			if generic || len(option.Args) != 2 || seenStatuses {
				r.Problems = append(r.Problems, r.Pos+": SuccessStatusRange requires two direct constant integers and one alternatives declaration")
				continue
			}
			bounds := [2]int64{}
			valid := true
			for i, expr := range option.Args {
				value := st.pass.TypesInfo.Types[expr].Value
				if value == nil || value.Kind() != constant.Int {
					valid = false
					break
				}
				var exact bool
				bounds[i], exact = constant.Int64Val(value)
				valid = valid && exact
			}
			if !valid || bounds[0] < 200 || bounds[1] > 299 || bounds[0] > bounds[1] {
				r.Problems = append(r.Problems, r.Pos+": SuccessStatusRange requires a constant range within 200..299")
				continue
			}
			seenStatuses = true
			for code := bounds[0]; code <= bounds[1]; code++ {
				r.SuccessStatuses = append(r.SuccessStatuses, int(code))
			}
		case "SuccessStatus", "SuccessStatuses":
			plural := obj.Name() == "SuccessStatuses"
			if generic || option.Ellipsis.IsValid() || len(option.Args) == 0 || !plural && len(option.Args) != 1 {
				r.Problems = append(r.Problems, r.Pos+": success status metadata requires direct constant integer arguments")
				continue
			}
			if !plural && seenStatus || plural && seenStatuses {
				r.Problems = append(r.Problems, r.Pos+": "+obj.Name()+" is declared more than once")
			}
			for _, expr := range option.Args {
				value := st.pass.TypesInfo.Types[expr].Value
				if value == nil || value.Kind() != constant.Int {
					r.Problems = append(r.Problems, r.Pos+": "+obj.Name()+" must be a constant integer")
					continue
				}
				code, exact := constant.Int64Val(value)
				redirect := code == 301 || code == 302 || code == 303 || code == 307 || code == 308
				if !exact || (code < 200 || code > 299) && (!redirect || plural) {
					r.Problems = append(r.Problems, r.Pos+": invalid "+obj.Name()+"; expected 2xx or a supported redirect status")
					continue
				}
				if plural {
					if slices.Contains(r.SuccessStatuses, int(code)) {
						r.Problems = append(r.Problems, r.Pos+": SuccessStatuses contains a repeated status")
					}
					r.SuccessStatuses = append(r.SuccessStatuses, int(code))
				} else {
					r.SuccessStatus = int(code)
				}
			}
			if plural {
				seenStatuses = true
			} else {
				seenStatus = true
			}
		case "ExternalError":
			if !generic || len(option.Args) != 0 {
				r.Problems = append(r.Problems, r.Pos+": external errors require direct web.ExternalError[E]() calls")
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
			// Registration uses schema.Compile, even though the response codec
			// follows encoding/json. Reject unsupported declarations before emit.
			st.recordProblems(rtshape.Describe(named, rtshape.Options{Qualifier: st.qualifier()}), named.Obj().Pos(), r)
			shape := rtshape.Describe(named, rtshape.Options{JSONOutput: true}).Root
			st.describeNamed(named, r)
			replaced := false
			for i := range r.Errors {
				if r.Errors[i].Key == shape.Key {
					r.Errors[i].Untagged = true
					replaced = true
					break
				}
			}
			if !replaced {
				r.Errors = append(r.Errors, ErrRef{Key: shape.Key, Name: shape.Name, Status: st.statusOf(named, r), Untagged: true})
			}
		default:
			r.Problems = append(r.Problems, r.Pos+": unsupported route option; "+routeOptionsHint)
		}
	}
	code := r.SuccessStatus
	redirect := code == 301 || code == 302 || code == 303 || code == 307 || code == 308
	switch {
	case r.NoContent && code != 204:
		r.Problems = append(r.Problems, r.Pos+": NoContent requires SuccessStatus(204)")
	case r.Redirect && !redirect:
		r.Problems = append(r.Problems, r.Pos+": RedirectResponse requires status 301, 302, 303, 307, or 308")
	case (r.Binary || r.Response || r.ProxyFailure != nil) && (code < 200 || code > 299):
		r.Problems = append(r.Problems, r.Pos+": typed dynamic responses require a 2xx SuccessStatus")
	case !r.NoContent && !r.Redirect && (code < 200 || code > 299 || code == 204 || code == 205) && !(r.Binary || r.Response || r.ProxyFailure != nil):
		r.Problems = append(r.Problems, r.Pos+": a response body requires a 2xx SuccessStatus other than 204 or 205")
	}
	if seenStatuses && r.ProxyFailure != nil {
		r.Problems = append(r.Problems, r.Pos+": ProxyResponse requires a single success status")
	} else if seenStatuses && !(r.Binary || r.Response) {
		r.Problems = append(r.Problems, r.Pos+": SuccessStatuses requires Response[T], FileResponse, or StreamResponse")
	}
	if !slices.Contains(r.SuccessStatuses, code) {
		r.SuccessStatuses = append(r.SuccessStatuses, code)
	}
	if r.File && slices.Contains(r.SuccessStatuses, 200) {
		for _, implicit := range []int{206, 304} {
			if !slices.Contains(r.SuccessStatuses, implicit) {
				r.SuccessStatuses = append(r.SuccessStatuses, implicit)
			}
		}
	}
	slices.Sort(r.SuccessStatuses)
}
