package effects

import (
	"fmt"
	"go/types"
	"slices"

	"github.com/effectgo/effectgo/internal/stubs"
)

// Effect stubs. The file format is shared with errset and is parsed by
// internal/stubs. This analyzer consumes the "effects" rules; the "errors"
// rules belong to errset and are ignored here (their syntax is still checked).

// stubEntry is the effects rule of one target: its fixed effects (empty means
// none) and its via items, which are evaluated at each call site (see via.go).
type stubEntry struct {
	effs   []string
	vias   []stubs.Via
	stores []int
}

// stubTable maps a target key to its effects rule.
type stubTable struct {
	effects map[string]stubEntry
}

// lookup returns the fixed stub effects for a target key. A nil table has no
// stubs.
func (t *stubTable) lookup(key string) ([]string, bool) {
	if t == nil {
		return nil, false
	}
	e, ok := t.effects[key]
	return e.effs, ok
}

// storesOf returns the argument indexes that the function for a target key
// keeps in the value it returns (stores items), or nil.
func (t *stubTable) storesOf(key string) []int {
	if t == nil {
		return nil
	}
	return t.effects[key].stores
}

// viaOf returns the via items of the rule for a target key, or nil.
func (t *stubTable) viaOf(key string) []stubs.Via {
	if t == nil {
		return nil
	}
	return t.effects[key].vias
}

// loadStubs builds the effects table from the stub rules of the analysis (see
// stubs.RulesForPass). Malformed rules are errors that name the file and line.
//
// Layers: a rule from a project file or an explicit -stubs file replaces a
// shipped rule (a stubs.IsShipped file) for the same target. Two rules of the
// same layer for one target are an error, and the error names the first one.
func loadStubs(rules []stubs.Rule) (*stubTable, error) {
	t := &stubTable{effects: map[string]stubEntry{}}
	first := map[string]stubs.Rule{}
	for _, r := range rules {
		if r.Verb != stubs.VerbEffects {
			continue
		}
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s:%d: %s", r.File, r.Line, fmt.Sprintf(format, a...))
		}
		key := r.Target.String()
		effs, vias, stores, err := stubs.ParseEffectsValue(r.Value)
		if err != nil {
			return nil, fail("%v", err)
		}
		for _, e := range effs {
			if !knownEffect[e] {
				return nil, fail("unknown effect name %q for %s", e, key)
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
		effs = slices.Compact(effs)
		t.effects[key] = stubEntry{effs: effs, vias: vias, stores: stores}
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
