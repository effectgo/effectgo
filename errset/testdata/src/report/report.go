package report

import (
	"domain"
	"errors"
	"fmt"
)

func A() error { return domain.ErrNotFound } // want "errors of A: domain.ErrNotFound" A:"."

func B() error { return fmt.Errorf("x %v", domain.ErrInvalid) } // want "errors of B: new@report.B" B:"."

func W() error { return fmt.Errorf("x: %w", domain.ErrInvalid) } // want "errors of W: domain.ErrInvalid" W:"."

func C() int { return 0 } // want C:"."

func D(n int) error { // want "errors of D: domain.ErrConflict \\| domain.ErrInvalid \\| domain.ErrNotFound" D:"."
	if n > 0 {
		return domain.ErrInvalid
	}
	return errors.Join(domain.ErrConflict, domain.ErrNotFound)
}

func H() func() error { // want "handler errors of H: domain.ErrConflict" H:"."
	return func() error { return domain.ErrConflict }
}
