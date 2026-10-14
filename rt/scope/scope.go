// Package scope closes a group of resources in the reverse order they were
// added, and reports every error that closing produced.
package scope

import (
	"errors"
	"io"
	"sync"
)

// Scope holds closers until Close. It is safe for concurrent use. The zero
// value is ready to use.
type Scope struct {
	mu      sync.Mutex
	closers []io.Closer
	late    []error // errors from closers added after Close
	closed  bool
}

// Add registers c to be closed by Close. A nil closer is ignored. If the
// scope is already closed, c is closed at once and its error is returned by
// the next Close, so no error is lost.
func (s *Scope) Add(c io.Closer) {
	if c == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		if err := c.Close(); err != nil {
			s.late = append(s.late, err)
		}
		return
	}
	s.closers = append(s.closers, c)
}

// Close closes the registered closers in reverse order of Add and returns
// all their errors joined (errors.Join), or nil. It removes the closers, so a
// second call returns only errors from closers added since. Every closer is
// closed even when an earlier one failed.
func (s *Scope) Close() error {
	s.mu.Lock()
	closers, late := s.closers, s.late
	s.closers, s.late = nil, nil
	s.closed = true
	s.mu.Unlock()

	errs := append([]error{}, late...)
	for i := len(closers) - 1; i >= 0; i-- {
		if err := closers[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
