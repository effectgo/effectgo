// Package app is the fixture: Huma routes over a project service.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"app/service"
)

type input struct {
	ID int `path:"id"`
}

type output struct {
	Body string
}

// mapErr is the project's mapper: it maps the validation error to 400, the
// sentinel ErrNotFound to 404, and everything else to a 500 of its own.
func mapErr(err error) error {
	var ve *service.ValidationError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ve):
		return huma.Error400BadRequest("invalid")
	case errors.Is(err, service.ErrNotFound):
		return huma.Error404NotFound("missing")
	default:
		return huma.Error500InternalServerError("unconfirmed")
	}
}

// getStatusErr implements huma's StatusError by GetStatus: the status is read by
// the framework, so the analyzer cannot know it.
type getStatusErr struct{}

func (*getStatusErr) Error() string  { return "gs" }
func (*getStatusErr) GetStatus() int { return http.StatusNotFound }

// codeErr has a StatusCode method, which Fuego reads and Huma does not.
type codeErr struct{}

func (*codeErr) Error() string   { return "code" }
func (*codeErr) StatusCode() int { return http.StatusNotFound }

// Register registers the routes of the fixture.
func Register(api huma.API, s service.Store, path string, status int) {
	huma.Get(api, "/raw-sentinel/{id}", func(ctx context.Context, in *input) (*output, error) {
		id, err := s.Find(in.ID)
		if err != nil {
			return nil, err
		}
		return &output{Body: fmt.Sprint(id)}, nil
	})

	huma.Post(api, "/save/{id}", func(ctx context.Context, in *input) (*output, error) {
		if err := s.Save(in.ID); err != nil {
			return nil, err
		}
		return &output{}, nil
	})

	huma.Get(api, "/mapped/{id}", func(ctx context.Context, in *input) (*output, error) {
		id, err := s.Find(in.ID)
		if err != nil {
			return nil, mapErr(err)
		}
		return &output{Body: fmt.Sprint(id)}, nil
	})

	huma.Put(api, "/wrapped-status", func(ctx context.Context, in *input) (*output, error) {
		return nil, fmt.Errorf("wrap: %w", huma.Error404NotFound("x"))
	})

	huma.Delete(api, "/fresh", func(ctx context.Context, in *input) (*output, error) {
		return nil, errors.New("boom")
	})

	huma.Get(api, path, func(ctx context.Context, in *input) (*output, error) {
		return &output{}, nil
	})

	huma.Register(api, huma.Operation{Method: http.MethodPatch, Path: "/register-literal"}, func(ctx context.Context, in *input) (*output, error) {
		return nil, service.ErrNotFound
	})

	op := huma.Operation{Method: http.MethodPost, Path: "/register-variable"}
	huma.Register(api, op, func(ctx context.Context, in *input) (*output, error) {
		return nil, service.ErrNotFound
	})

	huma.Get(api, "/constant-status", func(ctx context.Context, in *input) (*output, error) {
		return nil, huma.NewError(http.StatusConflict, "constant")
	})

	huma.Get(api, "/dynamic-status", func(ctx context.Context, in *input) (*output, error) {
		return nil, huma.NewError(status, "dynamic")
	})

	huma.Get(api, "/get-status", func(ctx context.Context, in *input) (*output, error) {
		return nil, &getStatusErr{}
	})

	huma.Get(api, "/status-code", func(ctx context.Context, in *input) (*output, error) {
		return nil, &codeErr{}
	})

	// A typed error returned raw; the predicate IsBad, tested in another route
	// with a 400 return, maps it.
	huma.Get(api, "/predicate-raw", func(ctx context.Context, in *input) (*output, error) {
		if err := s.Check(in.ID); err != nil {
			return nil, err
		}
		return &output{}, nil
	})

	huma.Put(api, "/predicate-guard", func(ctx context.Context, in *input) (*output, error) {
		if err := s.Check(in.ID); service.IsBad(err) {
			return nil, huma.Error400BadRequest("bad")
		}
		return &output{}, nil
	})

	// A handler-local errors.Is guard that maps ErrConflict to 409, in a route
	// other than the one that returns ErrConflict raw.
	huma.Put(api, "/conflict-guard", func(ctx context.Context, in *input) (*output, error) {
		if err := s.Save(in.ID); errors.Is(err, service.ErrConflict) {
			return nil, huma.Error409Conflict("taken")
		}
		return &output{}, nil
	})
}

type asTypeMappedError struct{}
type asTypeInlineError struct{}
type asTypeIgnoredError struct{}
type asTypeUnrelatedError struct{}

func (*asTypeMappedError) Error() string    { return "mapped" }
func (*asTypeInlineError) Error() string    { return "inline" }
func (*asTypeIgnoredError) Error() string   { return "ignored" }
func (*asTypeUnrelatedError) Error() string { return "unrelated" }

func mapAsType(err error) error {
	if _, ok := errors.AsType[*asTypeMappedError](err); ok {
		return huma.Error400BadRequest("mapped")
	}
	return huma.Error500InternalServerError("unmapped")
}

func inlineAsType(err error) (*output, error) {
	if _, ok := errors.AsType[*asTypeInlineError](err); ok {
		return nil, huma.Error409Conflict("inline")
	}
	return nil, err
}

func ignoresAsType(err error) bool {
	_, _ = errors.AsType[*asTypeIgnoredError](err)
	return true
}

func ignoredAsTypeGuard(err error) (*output, error) {
	if ignoresAsType(err) {
		return nil, huma.Error400BadRequest("ignored")
	}
	return nil, err
}

func unrelatedAsTypeGuard(err error, allowed bool) (*output, error) {
	if _, _ = errors.AsType[*asTypeUnrelatedError](err); allowed {
		return nil, huma.Error400BadRequest("unrelated")
	}
	return nil, err
}
