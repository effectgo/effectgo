package fresh

import (
	"domain"
	"errors"
	"fmt"
)

// ErrSentinel is a package-level error var: a sentinel, not a fresh error.
var ErrSentinel = errors.New("sentinel")

func Inline() error { return errors.New("inline") } // want "errors of Inline: new@fresh.Inline" Inline:"."

func Local() error { // want "errors of Local: new@fresh.Local" Local:"."
	err := errors.New("local")
	return err
}

func Errorf(n int) error { return fmt.Errorf("plain %d", n) } // want "errors of Errorf: new@fresh.Errorf" Errorf:"."

func Wrapped(e error) error { return fmt.Errorf("x: %w", e) } // want "errors of Wrapped: \\?" Wrapped:"."

func Dynamic(f string) error { return fmt.Errorf(f) } // want "errors of Dynamic: \\?" Dynamic:"."

func Sentinel() error { return ErrSentinel } // want "errors of Sentinel: fresh.ErrSentinel" Sentinel:"."

func Mixed(n int) error { // want "errors of Mixed: domain.ErrNotFound \\| new@fresh.Mixed" Mixed:"."
	if n > 0 {
		return domain.ErrNotFound
	}
	return errors.New("other")
}

func Helper() error { return helper() } // want "errors of Helper: new@fresh.helper" Helper:"."

func helper() error { return errors.New("h") } // want "errors of helper: new@fresh.helper" helper:"."

func Nested() error { return fmt.Errorf("w: %w", errors.New("nested")) } // want "errors of Nested: new@fresh.Nested" Nested:"."

// Only "new" allows fresh errors; "?" does not.

//effectgo:errors new
func AllowNew() error { return errors.New("ok") } // want "errors of AllowNew: new@fresh.AllowNew" AllowNew:"."

//effectgo:errors ?
func AllowOpaqueNotFresh() error { return errors.New("bad") } // want "errors of AllowOpaqueNotFresh: new@fresh.AllowOpaqueNotFresh" "AllowOpaqueNotFresh may return new@fresh.AllowOpaqueNotFresh, not in its //effectgo:errors contract" AllowOpaqueNotFresh:"."

//effectgo:errors new
func NewNotOpaque(e error) error { return fmt.Errorf("x: %w", e) } // want "errors of NewNotOpaque: \\?" "NewNotOpaque may return \\?, not in its //effectgo:errors contract" NewNotOpaque:"."

//effectgo:errors new, ?
func Both(e error) error { // want "errors of Both: new@fresh.Both \\| \\?" Both:"."
	if e == nil {
		return errors.New("fresh")
	}
	return fmt.Errorf("x: %w", e)
}

// Handlers: a fresh error is never covered by an errmap, so it is an
// unclassified error (HTTP 500).

//effectgo:errmap
func Map(err error) error { // want "errors of Map: domain.ErrNotFound \\| \\?" Map:"." Map:"."
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrNotFound
	}
	return err
}

//effectgo:handler Map
func Serve() error { // want "errors of Serve: new@fresh.Serve" "handler Serve may return fresh error new@fresh.Serve, .*HTTP 500" Serve:"."
	return errors.New("serve")
}
