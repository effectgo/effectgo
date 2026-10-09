// Command effectgo runs the effectgo analyzers as a multichecker. It works
// standalone and as a go vet tool:
//
//	effectgo ./...                                  effects, errset, mustuse and exhaustive diagnostics
//	effectgo -stubs=stubs/stdlib.stubs ./...        the same, with stub files
//	effectgo sig ./...                              inferred Effect<A, E, R> signatures
//	effectgo openapi [-merge|-check spec.json] ./... OpenAPI error responses of the routes
//	effectgo http-gin [-json] [-all] [-mappers] ./... Gin responses against the error sets
//	effectgo http-huma [-json] [-all] ./...         Huma routes: variants that reach HTTP 500
//	effectgo http [-json] [-all] ./...              the gin and Huma adapters over one load, one list
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
	"github.com/faintghost/effectgo/exhaustive"
	"github.com/faintghost/effectgo/internal/stubs"
	"github.com/faintghost/effectgo/mustuse"
	"github.com/faintghost/effectgo/signature"
)

func init() {
	// Registered on the global flag set so that go vet advertises it (go vet
	// forwards only the flags that the tool lists with -flags).
	flag.StringVar(&stubs.Flag, "stubs", "",
		"comma-separated stub files read by effects and errset; when empty, $"+stubs.Env+" is used, and when that is empty too, the shipped stubs plus the module's effectgo.stubs")
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
	if len(os.Args) > 1 && os.Args[1] == "http-gin" {
		os.Exit(runHTTPGin(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "http-huma" {
		os.Exit(runHTTPHuma(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "http" {
		os.Exit(runHTTP(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "sig" {
		os.Args = append(os.Args[:1:1], os.Args[2:]...)
		os.Args = withoutTests(os.Args)
		os.Args = takeEngineFlag(os.Args)
		multichecker.Main(signature.Analyzer)
		return
	}
	if err := checkStubs(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "effectgo: %v\n", err)
		os.Exit(1)
	}
	os.Args = withoutTests(os.Args)
	multichecker.Main(effects.Analyzer, errset.Analyzer, mustuse.Analyzer, exhaustive.Analyzer)
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
// ID includes a hash of this executable and of every stub file the analysis
// reads from the working directory (stubs.Sources: the explicit -stubs or
// $EFFECTGO_STUBS files, or the zero-configuration set, which is the shipped
// files plus the project file effectgo.stubs of the module of the working
// directory). Editing any of them changes the ID and invalidates the cached
// vet results. The format is the development-branch form that cmd/go parses:
// the last field is buildID=<hex>.
//
// Limitations: go vet runs -V=full without flags, so an explicit -stubs value
// is not known here (name the file in $EFFECTGO_STUBS, or run go vet with -a).
// Project files of other modules, below the working directory, are not hashed;
// they are picked up when the working directory is inside their module.
func printIdentity() {
	cwd, _ := os.Getwd()
	fmt.Printf("effectgo version devel buildID=%s\n", identityHash(cwd))
}

// identityHash is the hex digest of the executable and of the stub files that
// an analysis run from dir reads (see printIdentity).
func identityHash(dir string) string {
	h := sha256.New()
	if exe, err := os.Executable(); err == nil {
		if f, err := os.Open(exe); err == nil {
			_, _ = io.Copy(h, f)
			f.Close()
		}
	}
	srcs, err := stubs.Sources(dir)
	if err != nil {
		fmt.Fprintf(h, "unreadable %v\n", err)
	}
	for _, s := range srcs {
		fmt.Fprintf(h, "stub %q\n", s.Name)
		h.Write(s.Data)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// errsetEngineUsage is the usage line of -errset.engine.
const errsetEngineUsage = "error-set engine: ast (flow-insensitive) or ssa (flow-sensitive, SSA values); see errset"

// engineFlag is the -errset.engine flag of the subcommands that run the errset
// analyzer. It sets the engine of that analyzer, which the analysis reads.
type engineFlag struct{}

func (engineFlag) String() string { return errset.Analyzer.Flags.Lookup("engine").Value.String() }

func (engineFlag) Set(v string) error { return errset.Analyzer.Flags.Set("engine", v) }

// takeEngineFlag applies a -errset.engine argument of the sig subcommand and
// removes it from the arguments: multichecker would prefix an analyzer flag
// with the analyzer's name, so the flag is read here instead.
func takeEngineFlag(args []string) []string {
	out := args[:1:1]
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-errset.engine" || a == "--errset.engine":
			if i+1 < len(args) {
				i++
				_ = engineFlag{}.Set(args[i])
			}
		case strings.HasPrefix(a, "-errset.engine=") || strings.HasPrefix(a, "--errset.engine="):
			_ = engineFlag{}.Set(a[strings.Index(a, "=")+1:])
		default:
			out = append(out, a)
		}
	}
	return out
}

// bindEngine adds -errset.engine to a subcommand's flag set.
func bindEngine(fs *flag.FlagSet) {
	fs.Var(engineFlag{}, "errset.engine", errsetEngineUsage)
}
