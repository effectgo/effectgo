// Command effectgo runs the effectgo analyzers as a multichecker. It works
// standalone and as a go vet tool:
//
//	effectgo ./...                                  effects and errset diagnostics
//	effectgo -stubs=stubs/stdlib.stubs ./...        the same, with stub files
//	effectgo sig ./...                              inferred Effect<A, E, R> signatures
//	effectgo openapi [-merge|-check spec.json] ./... OpenAPI error responses of the routes
//	go vet -vettool=$(which effectgo) ./...         go vet, cached correctly (see identity)
//
// Flags: -stubs (shared by effects and errset; falls back to $EFFECTGO_STUBS),
// -effects.report, -effects.witness, -errset.report, -errset.witness,
// -errset.ignoreopaque, and -test (default false: test files are not analyzed;
// -test=true restores them). Under go vet the analyzer flags are forwarded the
// same way; go vet itself decides which packages and test variants it passes
// to the tool, so -test has no effect there.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/faintghost/effectgo/effects"
	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
	"github.com/faintghost/effectgo/signature"
)

func init() {
	// Registered on the global flag set so that go vet advertises it (go vet
	// forwards only the flags that the tool lists with -flags).
	flag.StringVar(&stubs.Flag, "stubs", "",
		"comma-separated stub files read by effects and errset; falls back to $"+stubs.Env+" when empty")
}

// testsFlag is the driver's flag that includes test files. multichecker
// registers it inside Main, so it cannot be set beforehand; instead the default
// is changed by putting -test=false in front of the arguments of a standalone
// run (see withoutTests).
const testsFlag = "-test"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "-V=full" {
		printIdentity()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "openapi" {
		os.Exit(runOpenAPI(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "sig" {
		os.Args = append(os.Args[:1:1], os.Args[2:]...)
		os.Args = withoutTests(os.Args)
		multichecker.Main(signature.Analyzer)
		return
	}
	os.Args = withoutTests(os.Args)
	multichecker.Main(effects.Analyzer, errset.Analyzer)
}

// withoutTests makes a standalone run skip test files unless -test is given:
// it puts -test=false first, so the flag parser sees it before the package
// patterns. A go vet invocation is left alone (it sends -flags, -V=full, or a
// .cfg file, and go vet decides the test variants itself).
func withoutTests(args []string) []string {
	if len(args) < 2 {
		return args
	}
	for _, a := range args[1:] {
		switch {
		case a == "-flags", strings.HasPrefix(a, "-V"), strings.HasSuffix(a, ".cfg"):
			return args
		case a == testsFlag, strings.HasPrefix(a, testsFlag+"="),
			a == "--"+testsFlag[1:], strings.HasPrefix(a, "--"+testsFlag[1:]+"="):
			return args // the user chose
		}
	}
	return append([]string{args[0], testsFlag + "=false"}, args[1:]...)
}

// printIdentity answers "effectgo -V=full", which go vet runs to get a tool ID.
// go vet caches results under that ID and does not look at stub files, so the
// ID includes a hash of this executable and of every stub file named by
// $EFFECTGO_STUBS. Editing a stub file then changes the ID and invalidates the
// cached vet results. The format is the development-branch form that cmd/go
// parses: the last field is buildID=<hex>.
//
// Limitation: -stubs flag values reach the tool only after this ID is computed
// (go vet runs -V=full without flags), so an edit to a file named only by
// -stubs is not in the ID and a cached result is reused. Either name the file in
// $EFFECTGO_STUBS as well, or run go vet with -a, which re-runs the analysis.
func printIdentity() {
	h := sha256.New()
	if exe, err := os.Executable(); err == nil {
		if f, err := os.Open(exe); err == nil {
			_, _ = io.Copy(h, f)
			f.Close()
		}
	}
	for _, p := range stubs.Paths() {
		data, err := os.ReadFile(p)
		fmt.Fprintf(h, "stub %q\n", p)
		if err != nil {
			fmt.Fprintf(h, "unreadable %v\n", err)
			continue
		}
		h.Write(data)
	}
	fmt.Printf("effectgo version devel buildID=%x\n", h.Sum(nil))
}
