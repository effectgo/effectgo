// Package plugin is the golangci-lint module plugin of effectgo: it exposes the
// contract analyzers (effects, errset) to golangci-lint v2, and through it to
// editors. It is built into a golangci-lint binary with `golangci-lint custom`
// (see docs/editor/ and README "Editor integration"); the report-only flags
// and the HTTP subcommands are not part of it.
//
// Configuration (linters.settings.custom.effectgo.settings in .golangci.yml):
//
//	stubs:        optional list of stub files. Empty or absent: the zero
//	              configuration of internal/stubs (the shipped stubs, plus the
//	              module's effectgo.stubs). Relative paths resolve against the
//	              module root of the working directory.
//	ignoreOpaque: do not report opaque ("?") and fresh (new@) errors in handler
//	              checks (errset -ignoreopaque). Default false.
//	mustuseAllow: callees whose discarded results mustuse does not report, by
//	              full name, e.g. "(*os.File).Close" (mustuse -mustuse.allow).
//	              Default none.
//
// Report flags (-effects.report, -errset.report and the witness flags) are
// never set: golangci-lint reports diagnostics, and the report lines are not
// diagnostics.
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/faintghost/effectgo/effects"
	"github.com/faintghost/effectgo/errset"
	"github.com/faintghost/effectgo/exhaustive"
	"github.com/faintghost/effectgo/internal/stubs"
	"github.com/faintghost/effectgo/mustuse"
)

// analyzers is the set of analyzers that the editors run. To expose another
// analyzer to editors, add it to this list; nothing else needs to change.
var analyzers = []*analysis.Analyzer{
	effects.Analyzer,
	errset.Analyzer,
	mustuse.Analyzer,
	exhaustive.Analyzer,
}

// Name is the plugin name: the key under linters.settings.custom, and the
// linter name that golangci-lint enables and prints.
const Name = "effectgo"

func init() {
	register.Plugin(Name, New)
}

// Settings are the plugin settings of .golangci.yml, decoded strictly: an
// unknown key is an error.
type Settings struct {
	Stubs        []string `json:"stubs"`
	IgnoreOpaque bool     `json:"ignoreOpaque"`
	MustuseAllow []string `json:"mustuseAllow"`
}

// Plugin is the golangci-lint module plugin of effectgo.
type Plugin struct {
	analyzers []*analysis.Analyzer
}

// New builds the plugin from the raw settings of golangci-lint. It applies the
// settings to the analyzers' shared state (the stub files and one flag); the
// analyzers are process-wide, so this runs once per golangci-lint run.
func New(conf any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[Settings](conf)
	if err != nil {
		return nil, err
	}
	if len(s.Stubs) > 0 {
		paths, err := resolveStubs(s.Stubs)
		if err != nil {
			return nil, err
		}
		stubs.Flag = strings.Join(paths, ",")
	}
	if err := errset.Analyzer.Flags.Set("ignoreopaque", fmt.Sprint(s.IgnoreOpaque)); err != nil {
		return nil, fmt.Errorf("effectgo: ignoreOpaque: %w", err)
	}
	if err := mustuse.Analyzer.Flags.Set("allow", strings.Join(trimAll(s.MustuseAllow), ",")); err != nil {
		return nil, fmt.Errorf("effectgo: mustuseAllow: %w", err)
	}
	return &Plugin{analyzers: analyzers}, nil
}

// BuildAnalyzers returns the analyzers of the plugin.
func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return p.analyzers, nil
}

// GetLoadMode asks for type information. The contract analyzers need the types
// of every package and the facts of the dependencies, which the syntax-only
// mode does not provide.
func (p *Plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

// trimAll drops blank entries and surrounding space from a list setting.
func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// resolveStubs makes the configured stub paths absolute. Relative paths are
// resolved against the module root of the working directory (the nearest
// go.mod), or the working directory when there is no module.
func resolveStubs(paths []string) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("effectgo: stubs: %w", err)
	}
	base := stubs.ModuleRoot(cwd)
	if base == "" {
		base = cwd
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("effectgo: stubs: every entry is blank")
	}
	return out, nil
}
