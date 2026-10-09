package stubs

// The value of an effects rule. Besides effect names and "pure", a value may
// carry via items, which name the argument of the call whose method performs
// effects:
//
//	effects fmt.Fprintf via:0.Write            # calling Write on argument 0
//	effects io.Copy via:0.Write,via:1.Read     # fixed effects may be combined
//
// The effects analyzer evaluates a via item at each call site, from the static
// type of the argument (see effects/via.go). This file only reads the syntax.

import (
	"fmt"
	"go/token"
	"strconv"
	"strings"
)

// Via is one "via:<argIndex>.<Method>" or "via:stored.<Method>" item of an
// effects value. The first calls Method on the argument with the 0-based index
// ArgIdx of the call. The second (Stored) calls Method on the value that the
// receiver stores: the argument that the receiver's constructor stored (see
// Stores).
type Via struct {
	ArgIdx int
	Method string
	Stored bool
}

// String renders the item in stub syntax.
func (v Via) String() string {
	if v.Stored {
		return "via:stored." + v.Method
	}
	return fmt.Sprintf("via:%d.%s", v.ArgIdx, v.Method)
}

// ParseEffectsValue reads the value of an effects rule: "pure", or a
// comma-separated list of effect names, via items and stores items. A stores
// item, "stores:<argIndex>", says that the function keeps its argument with that
// index in the value it returns (json.NewEncoder stores its writer). It checks
// the syntax of the items only; whether an effect name is known is the effects
// analyzer's check. "pure" stands alone, and an empty item is an error.
func ParseEffectsValue(s string) (effs []string, vias []Via, stores []int, err error) {
	if s == "pure" {
		return nil, nil, nil, nil
	}
	for _, tok := range strings.Split(s, ",") {
		switch {
		case tok == "":
			return nil, nil, nil, fmt.Errorf("effects value %q has an empty item", s)
		case tok == "pure":
			return nil, nil, nil, fmt.Errorf("effects value %q: pure cannot be combined with other items", s)
		case strings.HasPrefix(tok, "via:"):
			v, err := parseVia(tok)
			if err != nil {
				return nil, nil, nil, err
			}
			vias = append(vias, v)
		case strings.HasPrefix(tok, "stores:"):
			n, err := strconv.Atoi(strings.TrimPrefix(tok, "stores:"))
			if err != nil || n < 0 {
				return nil, nil, nil, fmt.Errorf("stores item %q: want stores:<argIndex> with a non-negative argument index", tok)
			}
			stores = append(stores, n)
		default:
			effs = append(effs, tok)
		}
	}
	return effs, vias, stores, nil
}

// parseVia reads "via:<argIndex>.<Method>" or "via:stored.<Method>".
func parseVia(tok string) (Via, error) {
	rest := strings.TrimPrefix(tok, "via:")
	if m, ok := strings.CutPrefix(rest, "stored."); ok {
		if !token.IsIdentifier(m) {
			return Via{}, fmt.Errorf("via item %q: want via:stored.<Method> with a method name", tok)
		}
		return Via{ArgIdx: -1, Method: m, Stored: true}, nil
	}
	num, method, ok := strings.Cut(rest, ".")
	n, err := strconv.Atoi(num)
	if !ok || err != nil || n < 0 || !token.IsIdentifier(method) {
		return Via{}, fmt.Errorf("via item %q: want via:<argIndex>.<Method> or via:stored.<Method>, with a non-negative argument index and a method name", tok)
	}
	return Via{ArgIdx: n, Method: method}, nil
}
