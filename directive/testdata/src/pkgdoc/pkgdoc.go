// Package pkgdoc has directives in its package doc comment. The pure directive is
// reported by effects; the errors directive is read by no analyzer there.
//
//effectgo:pure
//effectgo:errors ErrX // want `//effectgo:errors is not read in a package doc comment: only //effectgo:package is valid there`
package pkgdoc
