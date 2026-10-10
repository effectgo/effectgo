package effects

import (
	"strings"
	"testing"

	"github.com/faintghost/effectgo/internal/stubs"
)

func parseRules(t *testing.T, name, data string) []stubs.Rule {
	t.Helper()
	rules, err := stubs.Parse(name, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

// TestLoadStubsLayers: a project (or explicit) rule replaces a shipped rule of
// the same target; two rules of one layer are an error that names the first.
func TestLoadStubsLayers(t *testing.T) {
	shipped := parseRules(t, stubs.ShippedPrefix+"stdlib.stubs", "effects (context.Context).Done pure\n")
	project := parseRules(t, "/mod/effectgo.stubs", "effects (context.Context).Done net\n")

	tab, err := loadStubs(append(append([]stubs.Rule{}, shipped...), project...))
	if err != nil {
		t.Fatalf("project over shipped: %v", err)
	}
	if got, _ := tab.lookup("(context.Context).Done"); len(got) != 1 || got[0] != "net" {
		t.Errorf("project rule does not replace the shipped one: %v", got)
	}

	twice := parseRules(t, "/mod/effectgo.stubs", "effects time.Now time\n\neffects time.Now pure\n")
	_, err = loadStubs(twice)
	if err == nil {
		t.Fatal("two project rules for one target: no error")
	}
	if want := "/mod/effectgo.stubs:3: duplicate effects rule for time.Now, first defined at /mod/effectgo.stubs:1"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}

	shippedTwice := parseRules(t, stubs.ShippedPrefix+"x.stubs", "effects time.Now pure\neffects time.Now time\n")
	if _, err := loadStubs(shippedTwice); err == nil || !strings.Contains(err.Error(), "first defined at stubs/x.stubs:1") {
		t.Errorf("two shipped rules: error %v", err)
	}
	if got := stubs.Where(stubs.ShippedPrefix+"stdlib.stubs", 61); got != "stubs/stdlib.stubs:61" {
		t.Errorf("Where = %q", got)
	}
}
