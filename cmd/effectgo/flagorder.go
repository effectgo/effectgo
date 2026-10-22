package main

// Flags after the package patterns.
//
// The flag package stops parsing at the first argument that is not a flag, so
// in `effectgo sig ./internal/store/ -errset.statuses` the flag is read as a
// package pattern. go/packages then fails with "malformed import path ...:
// leading dash", which does not name the cause. checkFlagOrder finds those flags
// before any package is loaded, and says where they belong. go vet parses its
// own flags, so the vettool path does not use this check.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// checkFlagOrder returns an error when a flag of args comes after a package
// pattern, and nil otherwise. cmd is the subcommand name, or "" for the
// standalone run. fs is the flag set that parsed args, and args are the
// arguments after the subcommand name.
func checkFlagOrder(cmd string, fs *flag.FlagSet, args []string) error {
	o, ok := splitFlagOrder(fs, args)
	if !ok || len(o.names) == 0 {
		return nil
	}
	prog := progName(cmd)
	undefined := undefinedIn(fs, o.names)
	if len(undefined) == 0 {
		return fmt.Errorf("effectgo: %s after the package patterns; flags must come before them: %s",
			flagList(o.names), o.command(prog))
	}
	msg := fmt.Sprintf("effectgo: %s after the package patterns, and %s does not define %s",
		flagList(o.names), prog, dashed(undefined))
	// The subcommands do not take the report flags of the analyzers. When the
	// standalone run takes all of them, name the command that does.
	if cmd != "" {
		sa := standaloneFlagSet()
		if parseSilently(sa, args) {
			if so, ok := splitFlagOrder(sa, args); ok && len(so.names) > 0 && len(undefinedIn(sa, so.names)) == 0 {
				msg += "; the standalone run does: " + so.command(progName(""))
			}
		}
	}
	return errors.New(msg)
}

// checkSigFlagOrder is checkFlagOrder for effectgo sig, whose flags the
// multichecker parses. A command line that the sig flag set cannot parse is left
// to the multichecker, which reports its own flag error.
func checkSigFlagOrder(args []string) error {
	fs := sigFlagSet()
	if !parseSilently(fs, args) {
		return nil
	}
	return checkFlagOrder("sig", fs, args)
}

// flagOrder is a command line split at its package patterns.
type flagOrder struct {
	before     []string // the arguments before the first pattern: flags and their values
	names      []string // the names of the flags after the first pattern, without dashes
	moved      []string // those flags as typed, with their values
	patterns   []string // the package patterns, in order
	terminated bool     // a "--" came after the first pattern
}

// splitFlagOrder splits args, which fs has parsed, into the arguments before the
// first pattern and the flags after it. It reports ok false when a "--" ended the
// flags before the first pattern: then every later argument is a pattern.
func splitFlagOrder(fs *flag.FlagSet, args []string) (flagOrder, bool) {
	tail := fs.Args()
	n := len(args) - len(tail)
	o := flagOrder{before: args[:n]}
	if n > 0 && args[n-1] == "--" {
		return o, false
	}
	for i := 0; i < len(tail); i++ {
		a := tail[i]
		switch {
		case o.terminated:
			o.patterns = append(o.patterns, a)
		case a == "--":
			o.terminated = true
		case len(a) > 1 && a[0] == '-':
			name, _, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
			o.names = append(o.names, name)
			o.moved = append(o.moved, a)
			if !hasValue && takesValue(fs, name) && i+1 < len(tail) {
				i++
				o.moved = append(o.moved, tail[i])
			}
		default:
			o.patterns = append(o.patterns, a)
		}
	}
	return o, true
}

// command is the command line that puts the flags before the patterns: the
// program, the arguments before the first pattern, the moved flags, and the
// patterns. A "--" that ended the flags goes right before the patterns.
func (o flagOrder) command(prog string) string {
	parts := append([]string{prog}, o.before...)
	parts = append(parts, o.moved...)
	if o.terminated {
		parts = append(parts, "--")
	}
	return strings.Join(append(parts, o.patterns...), " ")
}

// takesValue reports whether the flag name of fs takes the next argument as its
// value. An undefined flag takes none.
func takesValue(fs *flag.FlagSet, name string) bool {
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !b.IsBoolFlag()
}

// undefinedIn returns the names that fs does not define.
func undefinedIn(fs *flag.FlagSet, names []string) []string {
	var out []string
	for _, n := range names {
		if fs.Lookup(n) == nil {
			out = append(out, n)
		}
	}
	return out
}

// parseSilently parses args with fs and reports whether they parsed. The flag
// set is used only to find the flags of a command line, so it prints nothing.
func parseSilently(fs *flag.FlagSet, args []string) bool {
	fs.SetOutput(io.Discard)
	return fs.Parse(args) == nil
}

// progName is the name of the program in a message: "effectgo", or
// "effectgo <cmd>" for a subcommand.
func progName(cmd string) string {
	if cmd == "" {
		return "effectgo"
	}
	return "effectgo " + cmd
}

// flagList names flags in a message: "flag -a" or "flags -a, -b".
func flagList(names []string) string {
	if len(names) > 1 {
		return "flags " + dashed(names)
	}
	return "flag " + dashed(names)
}

// dashed writes flag names with their dash: "-a, -b".
func dashed(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "-" + n
	}
	return strings.Join(out, ", ")
}

// sigFlagSet is the flag set of effectgo sig: the flags that the multichecker
// defines for the signature analyzer, and -errset.engine, which takeEngineFlag
// reads from the arguments. It is not the set the command runs with (the
// multichecker defines those flags when it runs): it lets checkFlagOrder find
// the flags of a command line. TestFlagSetsMatchTheCommands compares the two.
func sigFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("effectgo sig", flag.ContinueOnError)
	fs.Bool("V", false, "")
	fs.Bool("all", false, "")
	fs.Int("c", 0, "")
	fs.String("cpuprofile", "", "")
	fs.String("debug", "", "")
	fs.Bool("diff", false, "")
	fs.Bool("fix", false, "")
	fs.Bool("flags", false, "")
	fs.Bool("json", false, "")
	fs.String("memprofile", "", "")
	fs.Bool("signature", false, "")
	fs.Bool("source", false, "")
	fs.String("stubs", "", "")
	fs.String("tags", "", "")
	fs.Bool("test", true, "")
	fs.String("trace", "", "")
	fs.Bool("v", false, "")
	fs.String("errset.engine", "", "")
	return fs
}

// standaloneFlagSet is the flag set of the standalone run, as registerStandaloneFlags
// defines it on the command line. TestFlagSetsMatchTheCommands compares the two.
func standaloneFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("effectgo", flag.ContinueOnError)
	fs.Bool("test", true, "")
	fs.Bool("json", false, "")
	fs.Int("c", -1, "")
	fs.String("cpuprofile", "", "")
	fs.String("memprofile", "", "")
	fs.String("stubs", "", "")
	for _, a := range standaloneAnalyzers {
		a.Flags.VisitAll(func(f *flag.Flag) {
			fs.Var(f.Value, a.Name+"."+f.Name, f.Usage)
		})
	}
	return fs
}
