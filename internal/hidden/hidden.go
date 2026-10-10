// Package hidden records the diagnostics that an analyzer suppressed as the
// consequence of another function's violation (see docs/reference/analyzers.md),
// so that the standalone driver can say how many there were. A suppressed
// diagnostic is keyed by its position and text, as the printed diagnostics are
// de-duplicated: a function in a package and in its test variant is one entry,
// and a package analyzed more than once is counted once.
package hidden

import (
	"go/types"
	"sync"
)

var (
	mu   sync.Mutex
	keys = map[string]map[*types.Package]map[string]bool{} // analyzer, package, key
)

// Add records one hidden diagnostic of analyzer name in package pkg. key is
// the position and text of the diagnostic that was not printed.
func Add(name string, pkg *types.Package, key string) {
	if pkg == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if keys[name] == nil {
		keys[name] = map[*types.Package]map[string]bool{}
	}
	if keys[name][pkg] == nil {
		keys[name][pkg] = map[string]bool{}
	}
	keys[name][pkg][key] = true
}

// Total returns the number of distinct hidden diagnostics of analyzer name over
// the packages pkgs.
func Total(name string, pkgs []*types.Package) int {
	mu.Lock()
	defer mu.Unlock()
	union := map[string]bool{}
	for _, p := range pkgs {
		for k := range keys[name][p] {
			union[k] = true
		}
	}
	return len(union)
}
