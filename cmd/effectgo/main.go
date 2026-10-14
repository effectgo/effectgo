// Command effectgo runs the effectgo analyzers as a multichecker. It works
// standalone and as a go vet tool:
//
//	effectgo ./...                                  effects, errset, mustuse, exhaustive, schematag and directive diagnostics
//	effectgo -stubs=stubs/stdlib.stubs ./...        the same, with stub files
//	effectgo sig ./...                              inferred Effect<A, E, R> signatures
//	effectgo openapi [-merge|-check spec.json] ./... OpenAPI error responses of the routes
//	effectgo ts [-o FILE] [-name NAME] ./...        TypeScript client (effect/http-api) of the routes
//	effectgo http-gin [-json] [-all] [-mappers] ./... Gin responses against the error sets
//	effectgo http-huma [-json] [-all] ./...         Huma routes: variants that reach HTTP 500
//	effectgo http [-json] [-all] ./...              the gin and Huma adapters over one load, one list
//	go vet -vettool=$(which effectgo) ./...         go vet, cached correctly (see identity)
//
// Flags: -stubs (shared by effects and errset; falls back to $EFFECTGO_STUBS),
// -effects.report, -effects.witness, -effects.all, -errset.report,
// -errset.witness, -errset.ignoreopaque, -errset.all, and -test (default true:
// test files are analyzed, as go vet does; -test=false skips them). The
// standalone run ends with a summary line on stderr; go vet does not print it.
// Under go vet the analyzer flags are forwarded the same way; go vet itself
// decides which packages and test variants it passes to the tool, so -test has
// no effect there.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/faintghost/effectgo/directive"
	"github.com/faintghost/effectgo/effects"
	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/exhaustive"
	"github.com/faintghost/effectgo/internal/stubs"
	"github.com/faintghost/effectgo/mustuse"
	"github.com/faintghost/effectgo/schematag"
	"github.com/faintghost/effectgo/signature"
)

func init() {
	// Registered on the global flag set so that go vet advertises it (go vet
	// forwards only the flags that the tool lists with -flags).
	flag.StringVar(&stubs.Flag, "stubs", "",
		"comma-separated stub files read by effects and errset; when empty, $"+stubs.Env+" is used, and when that is empty too, the shipped stubs plus the module's effectgo.stubs")
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "-V=full" {
		printIdentity()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "openapi" {
		os.Exit(runOpenAPI(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "ts" {
		os.Exit(runTS(os.Args[2:]))
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
		if err := checkSigFlagOrder(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Args = append(os.Args[:1:1], os.Args[2:]...)
		os.Args = takeEngineFlag(os.Args)
		multichecker.Main(signature.Analyzer)
		return
	}
	if isVetInvocation(os.Args) || isHelpInvocation(os.Args) {
		if err := checkStubs(os.Args); err != nil {
			fmt.Fprintf(os.Stderr, "effectgo: %v\n", err)
			os.Exit(1)
		}
		multichecker.Main(effects.Analyzer, errset.Analyzer, mustuse.Analyzer, exhaustive.Analyzer, schematag.Analyzer, directive.Analyzer)
		return
	}
	// The standalone run checks its stub files after its flags are parsed.
	os.Exit(runStandalone(os.Args))
}

// isHelpInvocation reports whether args ask for the help of the analyzers,
// which the multichecker prints.
func isHelpInvocation(args []string) bool {
	return len(args) > 1 && (args[1] == "help" || args[1] == "-help" || args[1] == "--help" || args[1] == "-h")
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
