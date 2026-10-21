// Package directive is the effectgo analyzer for where //effectgo: directives are
// written. Each analyzer reads a directive only at the declarations its rules name,
// so a directive anywhere else checks nothing and would be silently inert. This
// analyzer reports those directives once, in the combined binary, and it reports a
// comment that has the form of a directive but a space after the slashes.
//
// The sites that some analyzer reads (or reports itself) are the table of this
// package. It mirrors the readers: effects (parsePackageContract, funcContracts,
// exportInterfaceContracts), errset (run, checkDirectives, collectInterfaceContracts),
// mustuse (exportMarkedTypes, checkFuncDirectives) and exhaustive (exportSealed). A
// change to a reader's placement must change the table here too.
package directive

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/effectgo/effectgo/internal/dirparse"
)

// Analyzer is the directive analyzer.
var Analyzer = &analysis.Analyzer{
	Name: "directive",
	Doc:  "reports //effectgo: directives that no analyzer reads where they are written, and comments that look like directives but have a space after //",
	Run:  run,
}

// keywords are the directive keywords of the analyzers: pure, allow and package
// (effects), errors, errmap and handler (errset), and mustuse and sealed (mustuse and
// exhaustive).
var keywords = map[string]bool{
	"pure": true, "allow": true, "package": true,
	"errors": true, "errmap": true, "handler": true,
	"mustuse": true, "sealed": true,
}

// site is the kind of comment group a directive sits in. The zero value is siteOther.
type site int

const (
	siteOther       site = iota // a var or const declaration, a body, a literal, a struct field, a detached or trailing comment
	sitePackage                 // the doc comment of the package clause
	siteFunc                    // the doc comment of a function or method with a body
	siteFuncNoBody              // the doc comment of a function or method without a body
	siteIface                   // the doc comment of an interface type, or of an alias of one
	siteIfaceMethod             // the doc comment of a named method of an interface type
	siteType                    // the doc comment of a type declaration that is not an interface
	siteTypeShadow              // the doc comment of a one-type group, when the type has a doc comment of its own
	siteTypeGroup               // the doc comment of a group that declares several types
)

// spot is the site of a comment group and the name of the declaration it documents.
type spot struct {
	site site
	name string
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		inTest := strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
		sites := sitesOf(file)
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				checkComment(pass, sites[cg], inTest, c)
			}
		}
	}
	return nil, nil
}

// checkComment reports a directive that no analyzer reads at its site, and a comment
// with a space after the slashes that names a directive keyword.
func checkComment(pass *analysis.Pass, at spot, inTest bool, c *ast.Comment) {
	if kw, ok := spacedKeyword(c.Text); ok {
		pass.Reportf(c.Pos(), "// effectgo:%s is not a directive: remove the space after //", kw)
		return
	}
	kw, _, ok := dirparse.Parse(c.Text)
	if !ok {
		return
	}
	if placed(kw, at, inTest) {
		return
	}
	pass.Reportf(c.Pos(), "%s", message(kw, at, inTest))
}

// spacedKeyword returns the keyword of a comment such as "// effectgo:pure". The
// word after effectgo: must be a directive keyword, so prose that mentions
// effectgo: is not matched.
func spacedKeyword(text string) (string, bool) {
	rest, ok := strings.CutPrefix(text, "//")
	if !ok {
		return "", false
	}
	trimmed := strings.TrimLeft(rest, " \t")
	if len(trimmed) == len(rest) {
		return "", false
	}
	body, ok := strings.CutPrefix(trimmed, "effectgo:")
	if !ok {
		return "", false
	}
	words := dirparse.Words(body)
	if len(words) == 0 || !keywords[words[0]] {
		return "", false
	}
	return words[0], true
}

// sitesOf maps the doc comment groups of a file to their sites. A comment group that
// is not in the map is at siteOther.
func sitesOf(file *ast.File) map[*ast.CommentGroup]spot {
	sites := map[*ast.CommentGroup]spot{}
	if file.Doc != nil {
		sites[file.Doc] = spot{site: sitePackage}
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc == nil {
				continue
			}
			s := siteFunc
			if d.Body == nil {
				s = siteFuncNoBody
			}
			sites[d.Doc] = spot{site: s, name: declName(d)}
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				typeSites(sites, d)
			}
		}
	}
	return sites
}

// typeSites maps the doc comments of a type declaration. A spec's own doc is read by
// the analyzers of types. The doc of a group with one spec is read as the spec's doc
// when the spec has none, and by mustuse and exhaustive even when it has one. The doc
// of a group with several specs is read by none of them. A named method of an
// interface has its own doc comment; an embedded interface does not count.
func typeSites(sites map[*ast.CommentGroup]spot, d *ast.GenDecl) {
	for _, spec := range d.Specs {
		ts := spec.(*ast.TypeSpec)
		if ts.Doc != nil {
			sites[ts.Doc] = spot{site: typeSite(ts), name: ts.Name.Name}
		}
		it, ok := ts.Type.(*ast.InterfaceType)
		if !ok || it.Methods == nil {
			continue
		}
		for _, fld := range it.Methods.List {
			if fld.Doc == nil || len(fld.Names) == 0 {
				continue
			}
			sites[fld.Doc] = spot{site: siteIfaceMethod, name: ts.Name.Name + "." + fld.Names[0].Name}
		}
	}
	if d.Doc == nil || len(d.Specs) == 0 {
		return
	}
	first := d.Specs[0].(*ast.TypeSpec)
	switch {
	case len(d.Specs) > 1:
		sites[d.Doc] = spot{site: siteTypeGroup}
	case first.Doc != nil:
		sites[d.Doc] = spot{site: siteTypeShadow, name: first.Name.Name}
	default:
		sites[d.Doc] = spot{site: typeSite(first), name: first.Name.Name}
	}
}

// typeSite is the site of a type's own doc comment: an interface or not.
func typeSite(ts *ast.TypeSpec) site {
	if _, ok := ts.Type.(*ast.InterfaceType); ok {
		return siteIface
	}
	return siteType
}

// declName names a function as the diagnostics show it: F, or T.M for a method of T.
func declName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) != 1 {
		return fd.Name.Name
	}
	t := fd.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ix, ok := t.(*ast.IndexExpr); ok {
		t = ix.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

// placed reports whether an analyzer reads the directive keyword kw at the site, or
// reports it there itself, so that this analyzer stays silent. An unknown keyword is
// reported by effects in a package, function, interface or interface method doc.
func placed(kw string, at spot, inTest bool) bool {
	if !keywords[kw] {
		switch at.site {
		case sitePackage, siteFunc, siteFuncNoBody, siteIface, siteIfaceMethod:
			return true
		}
		return false
	}
	switch at.site {
	case sitePackage:
		// package is the keyword of this site. effects reports pure and allow here.
		return kw == "package" || kw == "pure" || kw == "allow"
	case siteFunc:
		// effects reads pure and allow and reports package; errset reads errors,
		// errmap and handler; mustuse reports mustuse on a function.
		return kw != "sealed"
	case siteFuncNoBody:
		// effects reads pure and allow and reports package; mustuse reports mustuse.
		// errset checks only the functions it has a body for.
		return kw == "pure" || kw == "allow" || kw == "package" || kw == "mustuse"
	case siteIface:
		// effects reads pure and allow and reports package; errset reads errors and
		// reports errmap and handler; mustuse reads the type; exhaustive reads sealed
		// in the non-test files only.
		return kw != "sealed" || !inTest
	case siteIfaceMethod:
		// effects reads pure, allow and errors (errset), and reports package; errset
		// reports errmap and handler. mustuse and exhaustive read types only.
		return kw != "mustuse" && kw != "sealed"
	case siteType, siteTypeShadow:
		// mustuse reads a named type (and reports an alias); exhaustive reads sealed on
		// any type it exports or reports, in the non-test files only.
		return kw == "mustuse" || (kw == "sealed" && !inTest)
	}
	return false
}

// message is the diagnostic for a directive that no analyzer reads at its site.
func message(kw string, at spot, inTest bool) string {
	switch {
	case kw == "package":
		return "//effectgo:package is only valid in a package doc comment"
	case at.site == sitePackage:
		return fmt.Sprintf("//effectgo:%s is not read in a package doc comment: only //effectgo:package is valid there", kw)
	case at.site == siteOther:
		return fmt.Sprintf("//effectgo:%s is not attached to a function, interface or type declaration and is not read", kw)
	case kw == "":
		return "empty //effectgo: directive is not read here"
	case !keywords[kw]:
		return fmt.Sprintf("unknown effectgo directive %q is not read here", kw)
	case kw == "sealed" && inTest && isTypeSite(at.site):
		return "//effectgo:sealed is not read in a _test.go file: sealed types are checked only in non-test files"
	case at.site == siteFuncNoBody && (kw == "errors" || kw == "errmap" || kw == "handler"):
		return fmt.Sprintf("//effectgo:%s on function %s is not read: %s has no body, and only functions with a body are checked", kw, at.name, at.name)
	case at.site == siteFunc || at.site == siteFuncNoBody:
		return fmt.Sprintf("//effectgo:%s on function %s is not read: %s applies to %s", kw, at.name, kw, appliesTo(kw))
	case at.site == siteIfaceMethod:
		return fmt.Sprintf("//effectgo:%s on method %s is not read: %s applies to %s", kw, at.name, kw, appliesTo(kw))
	case at.site == siteType:
		return fmt.Sprintf("//effectgo:%s on type %s is not read: %s applies to %s", kw, at.name, kw, appliesTo(kw))
	case at.site == siteTypeShadow:
		return fmt.Sprintf("//effectgo:%s is in the doc comment of a type group, and the type has a doc comment of its own: put it on the type's doc comment", kw)
	case at.site == siteTypeGroup:
		return fmt.Sprintf("//effectgo:%s is in the doc comment of a group of several types and is not read: put it on each type", kw)
	}
	return fmt.Sprintf("//effectgo:%s is not read here", kw)
}

// isTypeSite reports whether a site is the doc comment of a type declaration.
func isTypeSite(s site) bool {
	return s == siteIface || s == siteType || s == siteTypeShadow || s == siteTypeGroup
}

// appliesTo names the declarations a directive keyword is read on, for the messages.
func appliesTo(kw string) string {
	switch kw {
	case "mustuse":
		return "named type declarations"
	case "sealed":
		return "interfaces and named integer or string types"
	case "errmap", "handler":
		return "functions"
	}
	return "functions, interfaces and interface methods"
}
