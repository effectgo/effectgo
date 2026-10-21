package main

import (
	"strings"

	"github.com/effectgo/effectgo/effects"
	"github.com/effectgo/effectgo/internal/stubs"
)

// checkStubs validates, once and before the analyzers run, the stub files that
// a standalone run reads: the explicit -stubs (or $EFFECTGO_STUBS) files, or,
// in zero configuration, the shipped files and the effectgo.stubs of the module
// of the working directory and of each package pattern. Without this check a
// bad rule fails every package's analysis, and the driver then prints the error
// once per package plus a "failed prerequisites" line for each dependent.
//
// args are the command-line arguments (os.Args). A go vet invocation (-V, -flags,
// or a unit .cfg file) is not checked: go vet runs one process per package, and
// each process reports its own error.
func checkStubs(args []string) error {
	if isVetInvocation(args) {
		return nil
	}
	flagValue := ""
	var dirs []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-stubs" && i+1 < len(args):
			flagValue = args[i+1]
			i++
		case strings.HasPrefix(a, "-stubs="):
			flagValue = strings.TrimPrefix(a, "-stubs=")
		case strings.HasPrefix(a, "-"):
			// another flag: its value, if any, is not a package pattern
		default:
			dirs = append(dirs, patternDir(a))
		}
	}
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	// The flag is not parsed yet: read it from the arguments, and restore it.
	old := stubs.Flag
	stubs.Flag = flagValue
	defer func() { stubs.Flag = old }()

	seen := map[string]bool{}
	for _, d := range dirs {
		key := "explicit"
		if !stubs.Configured() {
			key = stubs.ModuleRoot(d)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		rules, err := stubs.RulesFor(d)
		if err != nil {
			return err
		}
		if err := effects.CheckStubs(rules); err != nil {
			return err
		}
	}
	return nil
}

// isVetInvocation reports whether args are a go vet call: the version or flag
// query, or a unit configuration file.
func isVetInvocation(args []string) bool {
	for _, a := range args[1:] {
		if a == "-flags" || strings.HasPrefix(a, "-V") || strings.HasSuffix(a, ".cfg") {
			return true
		}
	}
	return false
}

// patternDir is the directory that a package pattern names: "./..." is ".",
// "./pkg/..." is "./pkg", and a plain directory is itself.
func patternDir(p string) string {
	p = strings.TrimSuffix(p, "...")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "."
	}
	return p
}
