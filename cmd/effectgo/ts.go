package main

// The ts subcommand. tsgen.Collect loads the packages and runs the analyzer on
// them, so the routes of every package are written to one file.

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/effectgo/effectgo/tsgen"
)

// runTS implements `effectgo ts [-o FILE] [-name NAME] [packages]`. It returns the
// exit status: 0 on success, 1 when the packages do not load or the routes cannot
// be generated, 2 on a usage error.
func runTS(args []string) int {
	fs := flag.NewFlagSet("effectgo ts", flag.ContinueOnError)
	outPath := fs.String("o", "", "write the TypeScript file to this path (default: stdout)")
	name := fs.String("name", "", "the name of the API (default: the last element of the module path)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := checkFlagOrder("ts", fs, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	text, err := generateTS(patterns, *name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "effectgo ts: %v\n", err)
		return 1
	}
	if *outPath == "" {
		fmt.Print(text)
		return 0
	}
	if err := os.WriteFile(*outPath, []byte(text), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "effectgo ts: %v\n", err)
		return 1
	}
	return 0
}

// generateTS collects the routes of the packages and writes the file. The API
// name defaults to the last element of the module path.
func generateTS(patterns []string, api string) (string, error) {
	c, err := tsgen.Collect("", patterns...)
	if err != nil {
		return "", err
	}
	if api == "" {
		api = path.Base(c.Module)
	}
	return tsgen.Generate(c.Results, api, c.Module, c.Packages)
}
