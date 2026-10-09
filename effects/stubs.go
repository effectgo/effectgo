package effects

import (
	"fmt"
	"go/types"
	"slices"
	"strings"

	"github.com/faintghost/effectgo/internal/stubs"
)

// Effect stubs. The file format is shared with errset and is parsed by
// internal/stubs. This analyzer consumes the "effects" rules; the "errors"
// rules belong to errset and are ignored here (their syntax is still checked).

// stubTable maps a target key to its effects. An empty list means pure.
type stubTable struct {
	effects map[string][]string
}

// lookup returns the stub effects for a target key. A nil table has no stubs.
func (t *stubTable) lookup(key string) ([]string, bool) {
	if t == nil {
		return nil, false
	}
	effs, ok := t.effects[key]
	return effs, ok
}

// loadStubs builds the effects table from the stub rules of the analysis (see
// stubs.RulesForPass). Malformed rules are errors that name the file and line.
//
// Layers: a rule from a project file or an explicit -stubs file replaces a
// shipped rule (a stubs.IsShipped file) for the same target. Two rules of the
// same layer for one target are an error, and the error names the first one.
func loadStubs(rules []stubs.Rule) (*stubTable, error) {
	t := &stubTable{effects: map[string][]string{}}
	first := map[string]stubs.Rule{}
	for _, r := range rules {
		if r.Verb != stubs.VerbEffects {
			continue
		}
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s:%d: %s", r.File, r.Line, fmt.Sprintf(format, a...))
		}
		key := r.Target.String()
		var effs []string
		if r.Value != "pure" {
			for _, e := range strings.Split(r.Value, ",") {
				if !knownEffect[e] {
					return nil, fail("unknown effect name %q for %s", e, key)
				}
				if !slices.Contains(effs, e) {
					effs = append(effs, e)
				}
			}
		}
		if prev, dup := first[key]; dup {
			switch {
			case stubs.IsShipped(prev.File) && !stubs.IsShipped(r.File):
				// The project or explicit file replaces the shipped rule.
			case stubs.IsShipped(r.File) && !stubs.IsShipped(prev.File):
				continue // the project rule already replaced this shipped one
			default:
				return nil, fail("duplicate effects rule for %s, first defined at %s", key, stubs.Where(prev.File, prev.Line))
			}
		}
		first[key] = r
		slices.Sort(effs)
		t.effects[key] = effs
	}
	return t, nil
}

// CheckStubs reports the first error of the effects rules of rules: a
// duplicate in one layer, or an unknown effect name. A command runs it before
// the analyzers, so that the error is reported once rather than once per
// package (see cmd/effectgo).
func CheckStubs(rules []stubs.Rule) error {
	_, err := loadStubs(rules)
	return err
}

// targetKey is the key under which a stub for fn is looked up. It has the same
// form as a stub target: "pkg.Func", "(pkg.T).M", "(*pkg.T).M", or "(error).Error"
// for the predeclared type. Generic origins are used.
func targetKey(fn *types.Func) string {
	fn = fn.Origin()
	if fn.Pkg() == nil && !isIfaceOrMethod(fn) {
		return ""
	}
	if !isIfaceOrMethod(fn) {
		return fn.Pkg().Path() + "." + fn.Name()
	}
	ptr, owner := methodOwner(fn)
	if owner == nil {
		return ""
	}
	qual := owner.Name()
	if owner.Pkg() != nil {
		qual = owner.Pkg().Path() + "." + owner.Name()
	}
	star := ""
	if ptr {
		star = "*"
	}
	return "(" + star + qual + ")." + fn.Name()
}

func isIfaceOrMethod(fn *types.Func) bool {
	return fn.Type().(*types.Signature).Recv() != nil
}
