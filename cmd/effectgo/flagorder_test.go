package main

import (
	"flag"
	"io"
	"slices"
	"strings"
	"testing"
)

// The checks of flags after the package patterns (flagorder.go). The end-to-end
// cases run the built binary on a scratch module: each command exits 2 with the
// message, before any package is loaded.

// TestCheckFlagOrder covers the split of a command line, with a flag set of a
// subcommand that takes a bool, an int and a string flag.
func TestCheckFlagOrder(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string // the message; "" when no flag follows a pattern
	}{
		{"flags before the patterns", []string{"-json", "-c", "2", "./..."}, ""},
		{"no patterns", []string{"-json"}, ""},
		{"a bool flag after", []string{"./...", "-json"},
			"effectgo: flag -json after the package patterns; flags must come before them: effectgo http-gin -json ./..."},
		{"a value after", []string{"./...", "-c", "2"},
			"effectgo: flag -c after the package patterns; flags must come before them: effectgo http-gin -c 2 ./..."},
		{"an inline value after", []string{"./...", "-stubs=a.stubs"},
			"effectgo: flag -stubs after the package patterns; flags must come before them: effectgo http-gin -stubs=a.stubs ./..."},
		{"flags on both sides", []string{"-json", "./a", "-c", "2", "./b"},
			"effectgo: flag -c after the package patterns; flags must come before them: effectgo http-gin -json -c 2 ./a ./b"},
		{"two flags after", []string{"./...", "-json", "-c", "3"},
			"effectgo: flags -json, -c after the package patterns; flags must come before them: effectgo http-gin -json -c 3 ./..."},
		{"a flag after a terminator", []string{"./...", "-json", "--", "./b"},
			"effectgo: flag -json after the package patterns; flags must come before them: effectgo http-gin -json -- ./... ./b"},
		{"a terminator before the patterns", []string{"--", "-json"}, ""},
		{"a terminator after a pattern", []string{"./...", "--", "-json"}, ""},
		{"a lone dash is a pattern", []string{"./...", "-"}, ""},
		{"an undefined flag", []string{"./...", "-nosuch"},
			"effectgo: flag -nosuch after the package patterns, and effectgo http-gin does not define -nosuch"},
	}
	for _, c := range cases {
		fs := flag.NewFlagSet("effectgo http-gin", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.Bool("json", false, "")
		fs.Int("c", 0, "")
		fs.String("stubs", "", "")
		if err := fs.Parse(c.args); err != nil {
			t.Fatalf("%s: parse: %v", c.name, err)
		}
		got := ""
		if err := checkFlagOrder("http-gin", fs, c.args); err != nil {
			got = err.Error()
		}
		if got != c.want {
			t.Errorf("%s: args %q\n got  %q\n want %q", c.name, c.args, got, c.want)
		}
	}
}

// TestCheckFlagOrderSig covers the sig subcommand, whose flags are the
// multichecker's, and the standalone run, which takes the report flags.
func TestCheckFlagOrderSig(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"a report flag of the standalone run",
			[]string{"./internal/store/", "-errset.statuses"},
			"effectgo: flag -errset.statuses after the package patterns, and effectgo sig does not define -errset.statuses; the standalone run does: effectgo -errset.statuses ./internal/store/"},
		{"a flag that sig takes, with its value",
			[]string{"./internal/store/", "-errset.engine", "ast"},
			"effectgo: flag -errset.engine after the package patterns; flags must come before them: effectgo sig -errset.engine ast ./internal/store/"},
		{"a flag that sig takes before the patterns",
			[]string{"-errset.engine", "ast", "./internal/store/"}, ""},
		{"an undefined flag before the patterns is left to the multichecker",
			[]string{"-errset.statuses", "./internal/store/"}, ""},
	}
	for _, c := range cases {
		got := ""
		if err := checkSigFlagOrder(c.args); err != nil {
			got = err.Error()
		}
		if got != c.want {
			t.Errorf("%s: args %q\n got  %q\n want %q", c.name, c.args, got, c.want)
		}
	}

	// The standalone flag set takes -errset.statuses, so the message names no
	// other command; a flag that it does not take has no suggestion.
	sa := standaloneFlagSet()
	args := []string{"./x", "-errset.statuses"}
	if !parseSilently(sa, args) {
		t.Fatal("the standalone flag set does not parse the arguments")
	}
	err := checkFlagOrder("", sa, args)
	want := "effectgo: flag -errset.statuses after the package patterns; flags must come before them: effectgo -errset.statuses ./x"
	if err == nil || err.Error() != want {
		t.Errorf("standalone: got %v, want %q", err, want)
	}
}

// usageFlags reads the flags that the flag package lists after a usage error:
// each name, and whether it takes a value (a type name follows the name).
func usageFlags(stderr string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.HasPrefix(line, "  -") {
			continue
		}
		left, _, _ := strings.Cut(line, "\t")
		fields := strings.Fields(left)
		out[strings.TrimPrefix(fields[0], "-")] = len(fields) > 1
	}
	return out
}

// TestFlagSetsMatchTheCommands compares the mirror flag sets with the flags that
// the real commands list in their usage: the names, and which flags take a value.
func TestFlagSetsMatchTheCommands(t *testing.T) {
	bin := buildEffectgo(t)
	dir := t.TempDir()

	_, sigUsage, code := runIn(t, bin, dir, "sig", "-nosuch")
	if code != 2 {
		t.Fatalf("sig -nosuch: exit %d, want 2", code)
	}
	compareFlagSet(t, "sig", usageFlags(sigUsage), sigFlagSet(), "errset.engine")

	_, saUsage, code := runIn(t, bin, dir, "-nosuch", "./...")
	if code != 2 {
		t.Fatalf("-nosuch: exit %d, want 2", code)
	}
	compareFlagSet(t, "standalone", usageFlags(saUsage), standaloneFlagSet())
}

// compareFlagSet reports the differences between the flags of a usage listing
// (name -> takes a value) and a flag set. extra names are in the set only.
func compareFlagSet(t *testing.T, cmd string, listed map[string]bool, fs *flag.FlagSet, extra ...string) {
	t.Helper()
	if len(listed) == 0 {
		t.Fatalf("%s: no flags in the usage", cmd)
	}
	kinds := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		b, ok := f.Value.(interface{ IsBoolFlag() bool })
		kinds[f.Name] = !(ok && b.IsBoolFlag())
	})
	for name, value := range listed {
		got, ok := kinds[name]
		switch {
		case !ok:
			t.Errorf("%s lists -%s; the mirror flag set does not define it", cmd, name)
		case got != value:
			t.Errorf("%s: -%s takes a value: %v in the usage, %v in the mirror set", cmd, name, value, got)
		}
	}
	for name := range kinds {
		if _, ok := listed[name]; !ok && !slices.Contains(extra, name) {
			t.Errorf("the mirror flag set of %s defines -%s; the command does not list it", cmd, name)
		}
	}
}

// TestFlagAfterPatterns runs each command with a flag after its patterns: it
// exits 2 with the message, and the suggested command takes the flags first.
func TestFlagAfterPatterns(t *testing.T) {
	bin := buildEffectgo(t)
	dir := scratchModule(t, true)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"./...", "-effects.report"},
			"effectgo: flag -effects.report after the package patterns; flags must come before them: effectgo -effects.report ./..."},
		{[]string{"./...", "-stubs", "x.stubs", "-json"},
			"effectgo: flags -stubs, -json after the package patterns; flags must come before them: effectgo -stubs x.stubs -json ./..."},
		{[]string{"./...", "-nosuch"},
			"effectgo: flag -nosuch after the package patterns, and effectgo does not define -nosuch"},
		{[]string{"sig", "./", "-errset.statuses"},
			"effectgo: flag -errset.statuses after the package patterns, and effectgo sig does not define -errset.statuses; the standalone run does: effectgo -errset.statuses ./"},
		{[]string{"sig", "./", "-test=false"},
			"effectgo: flag -test after the package patterns; flags must come before them: effectgo sig -test=false ./"},
		{[]string{"openapi", "./...", "-merge", "spec.json"},
			"effectgo: flag -merge after the package patterns; flags must come before them: effectgo openapi -merge spec.json ./..."},
		{[]string{"http", "./...", "-json"},
			"effectgo: flag -json after the package patterns; flags must come before them: effectgo http -json ./..."},
		{[]string{"http-gin", "./...", "-mappers"},
			"effectgo: flag -mappers after the package patterns; flags must come before them: effectgo http-gin -mappers ./..."},
		{[]string{"http-huma", "./...", "-all"},
			"effectgo: flag -all after the package patterns; flags must come before them: effectgo http-huma -all ./..."},
	}
	for _, c := range cases {
		stdout, stderr, code := runIn(t, bin, dir, c.args...)
		if code != 2 || stdout != "" || stderr != c.want+"\n" {
			t.Errorf("effectgo %q: exit %d, stdout %q\n stderr %q\n want exit 2 and %q", c.args, code, stdout, stderr, c.want)
		}
	}
}

// TestFlagsBeforePatternsRun checks that the commands whose flags are in the
// right order are not stopped by the check.
func TestFlagsBeforePatternsRun(t *testing.T) {
	bin := buildEffectgo(t)
	dir := scratchModule(t, true)
	for _, args := range [][]string{
		{"-effects.report", "./..."},
		{"sig", "-test=false", "./"},
		{"http-gin", "-json", "./..."},
		{"http", "-json", "./..."},
		{"http-huma", "-json", "./..."},
		{"openapi", "./..."},
	} {
		_, stderr, _ := runIn(t, bin, dir, args...)
		if strings.Contains(stderr, "after the package patterns") {
			t.Errorf("effectgo %q: the check stopped a valid command:\n%s", args, stderr)
		}
	}
}
