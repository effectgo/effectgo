package witness

import (
	"domain"
	"errors"
)

func Direct() error { return domain.ErrNotFound } // want "witness of Direct: domain.ErrNotFound via \\(direct\\)" Direct:"."

func Chain() error { return Direct() } // want "witness of Chain: domain.ErrNotFound via Direct" Chain:"."

func Fresh() error { return errors.New("x") } // want "witness of Fresh: new@witness.Fresh via \\(direct\\)" Fresh:"."

func Deep() error { return Chain() } // want "witness of Deep: domain.ErrNotFound via Chain → Direct" Deep:"."

func Opaque(e error) error { return e } // want "witness of Opaque: \\? via \\(direct\\)" Opaque:"."

func Handler() func() error { // want "witness of Handler \\[handler\\]: domain.ErrConflict via \\(direct\\)" Handler:"."
	return func() error { return domain.ErrConflict }
}
