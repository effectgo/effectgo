package schematag

// The vocabulary is read from rtshape, which holds the copy of rt/schema's
// tables. These names remain for schematag_test.go, which compares them with
// rt/schema/schema.go; the analyzer itself uses rtshape.

import (
	"github.com/faintghost/effectgo/internal/rtshape"
)

// ruleSpec is one rule of a schema tag (rtshape.RuleSpec with unexported fields).
type ruleSpec struct {
	noArg bool
	cats  []string
}

// rules is the closed vocabulary of schema tags.
var rules = func() map[string]ruleSpec {
	out := map[string]ruleSpec{}
	for name, spec := range rtshape.Rules {
		out[name] = ruleSpec{noArg: spec.NoArg, cats: spec.Cats}
	}
	return out
}()

var kindCategory = rtshape.KindCategory

var supportedKinds = rtshape.SupportedKinds
