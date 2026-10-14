package tsgen

import (
	"fmt"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

// Collected is the output of Collect: the result of each root package, the
// paths of the packages that have routes, and the module.
type Collected struct {
	Results  []Result
	Packages []string
	Module   string
}

// Collect loads the packages of the patterns in dir (the working directory when
// empty), runs the tsgen analyzer on them, and gathers the results.
func Collect(dir string, patterns ...string) (Collected, error) {
	var c Collected
	cfg := &packages.Config{Mode: packages.LoadAllSyntax | packages.NeedModule, Dir: dir}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return c, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return c, fmt.Errorf("the packages do not load")
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{Analyzer}, pkgs, nil)
	if err != nil {
		return c, err
	}
	for _, act := range graph.Roots {
		if act.Err != nil {
			return c, fmt.Errorf("%s: %v", act.Package.PkgPath, act.Err)
		}
		if act.Package.Module != nil && c.Module == "" {
			c.Module = act.Package.Module.Path
		}
		r, ok := act.Result.(Result)
		if !ok {
			continue
		}
		c.Results = append(c.Results, r)
		if len(r.Routes) > 0 {
			c.Packages = append(c.Packages, act.Package.PkgPath)
		}
	}
	if c.Module == "" {
		return c, fmt.Errorf("the packages are not in a module")
	}
	return c, nil
}
