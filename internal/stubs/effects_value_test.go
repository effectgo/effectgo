package stubs

import (
	"strings"
	"testing"
)

// TestParseEffectsValue checks the value forms of an effects rule.
func TestParseEffectsValue(t *testing.T) {
	effs, vias, stores, err := ParseEffectsValue("pure")
	if err != nil || len(effs) != 0 || len(vias) != 0 || len(stores) != 0 {
		t.Fatalf("pure: %v %v %v %v", effs, vias, stores, err)
	}
	effs, vias, stores, err = ParseEffectsValue("net,fs,via:0.Write,via:1.Read,stores:0,via:stored.Write")
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 1 || stores[0] != 0 || len(vias) != 3 || !vias[2].Stored || vias[2].String() != "via:stored.Write" {
		t.Errorf("stores = %v, vias = %v", stores, vias)
	}
	vias = vias[:2]
	if strings.Join(effs, ",") != "net,fs" {
		t.Errorf("effects = %v", effs)
	}
	if len(vias) != 2 || vias[0] != (Via{ArgIdx: 0, Method: "Write"}) || vias[1].String() != "via:1.Read" {
		t.Errorf("vias = %v", vias)
	}
}

// TestMalformedEffectsValue: each malformed value is a parse error that names
// the file and the line.
func TestMalformedEffectsValue(t *testing.T) {
	cases := []struct {
		name, line, want string
	}{
		{"via without a dot", "effects fmt.Fprintf via:0Write\n", "x.stubs:1: via item \"via:0Write\""},
		{"via negative index", "effects fmt.Fprintf via:-1.Write\n", "x.stubs:1: via item \"via:-1.Write\""},
		{"via index not a number", "effects fmt.Fprintf via:a.Write\n", "x.stubs:1: via item \"via:a.Write\""},
		{"via method not an identifier", "effects fmt.Fprintf via:0.Write-now\n", "x.stubs:1: via item"},
		{"stored via on a function", "effects fmt.Fprintf via:stored.Write\n", "x.stubs:1: via:stored items need a method target"},
		{"stores on a method", "effects (*a.T).M stores:0\n", "x.stubs:1: stores items need a function target"},
		{"stores not a number", "effects a.New stores:x\n", "x.stubs:1: stores item \"stores:x\""},
		{"pure with a via", "\neffects fmt.Fprintf pure,via:0.Write\n", "x.stubs:2: effects value \"pure,via:0.Write\": pure cannot be combined"},
		{"empty item", "effects fmt.Fprintf net,,fs\n", "x.stubs:1: effects value \"net,,fs\" has an empty item"},
		{"via on a type target", "effects (io.Writer) via:0.Write\n", "x.stubs:1: via items need a function or a method target"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse("x.stubs", []byte(c.line))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}
