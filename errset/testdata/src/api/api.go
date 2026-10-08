package api

import (
	"domain"
	"httpres"
)

var _ = httpres.ToHTTPError

type Req struct{ Title string }

// v3Update may return ErrConflict via Service.Update -> Store.modify, which ToHTTPError does not map.
//
//effectgo:handler httpres.ToHTTPError
func v3Update(svc *domain.Service) func(Req) (domain.Todo, error) { // want "handler v3Update may return domain.ErrConflict" v3Update:"."
	return func(r Req) (domain.Todo, error) {
		return svc.Update(1, r.Title)
	}
}

// v3Get only returns ErrNotFound and ErrInvalid, both mapped: silent.
//
//effectgo:handler httpres.ToHTTPError
func v3Get(svc *domain.Service) func(Req) (domain.Todo, error) { // want v3Get:"."
	return func(r Req) (domain.Todo, error) {
		if r.Title == "" {
			return domain.Todo{}, domain.ErrInvalid
		}
		return svc.Get(1)
	}
}

// v3Check returns a *ValidationError, which StatusError covers.
//
//effectgo:handler httpres.ToHTTPError
func v3Check(svc *domain.Service) func(Req) error { // want v3Check:"."
	return func(r Req) error {
		return svc.Check(r.Title)
	}
}

//effectgo:handler httpres.NoSuchMapper
func v3Missing() func() error { return func() error { return nil } } // want "cannot resolve \"httpres.NoSuchMapper\"" v3Missing:"."
