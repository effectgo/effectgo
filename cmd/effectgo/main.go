// Command effectgo runs the effects and errset analyzers as a multichecker.
// It works standalone (effectgo ./...) and as a go vet tool
// (go vet -vettool=$(which effectgo) ./...).
package main

import (
	"flag"

	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/faintghost/effectgo/effects"
	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/internal/stubs"
)

func init() {
	// Registered on the global flag set so that go vet advertises it.
	flag.StringVar(&stubs.Flag, "stubs", "",
		"comma-separated stub files read by effects and errset; falls back to $"+stubs.Env+" when empty")
}

func main() {
	multichecker.Main(effects.Analyzer, errset.Analyzer)
}
