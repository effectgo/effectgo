// Package passthrough is an errmap whose only rule returns the error unchanged.
// A passthrough keeps the status that each variant carries (errors.md, HTTP
// status): a variant with a constant StatusCode is handled by the errmap, and
// a variant without a status of its own is sent as 500, so it is still reported.
package passthrough

import "errors"

// ErrPlain is a sentinel: a plain error with no status.
var ErrPlain = errors.New("plain")

// Conflict has a constant StatusCode: 409.
type Conflict struct{}

func (Conflict) Error() string   { return "conflict" } // want Error:"."
func (Conflict) StatusCode() int { return 409 }        // want StatusCode:"." StatusCode:"."

// Plain has no StatusCode method: it is a plain error.
type Plain struct{}

func (Plain) Error() string { return "plain" } // want Error:"."

// Dyn's status depends on its field, so it has no constant status.
type Dyn struct{ Code int }

func (e Dyn) Error() string { return "dyn" } // want Error:"."
func (e Dyn) StatusCode() int { // want StatusCode:"."
	if e.Code == 0 {
		return 500
	}
	return e.Code
}

// PassMap returns every error unchanged.
//
//effectgo:errmap
func PassMap(err error) error { // want PassMap:"." PassMap:"."
	return err
}

// Handled returns a variant with a constant status: the passthrough keeps 409,
// so the handler is not reported.
//
//effectgo:handler PassMap
func Handled() error { // want Handled:"."
	return Conflict{}
}

// Unhandled returns a plain error: the passthrough keeps no status, so it is
// reported as 500.
//
//effectgo:handler PassMap
func Unhandled() error { // want Unhandled:"." "does not map"
	return Plain{}
}

// Sentinel returns a sentinel: no status of its own, reported as 500.
//
//effectgo:handler PassMap
func Sentinel() error { // want Sentinel:"." "does not map"
	return ErrPlain
}

// Dynamic returns a variant whose status is not a constant: it is not kept by
// the passthrough, and it is reported.
//
//effectgo:handler PassMap
func Dynamic(code int) error { // want Dynamic:"." "does not map"
	return Dyn{Code: code}
}

// Coded carries its status in a field, as the httpError of the errmap docs
// does; it has no StatusCode method.
type Coded struct {
	Status int
	Err    error
}

func (Coded) Error() string { return "coded" } // want Error:"."

// Fallback maps ErrPlain to Conflict and every other error to Coded with 422.
// Its last rule is an unconditional return with a constant status, so every
// variant that no earlier rule matches is sent with 422.
//
//effectgo:errmap
func Fallback(err error) error { // want Fallback:"." Fallback:"."
	if errors.Is(err, ErrPlain) {
		return Conflict{}
	}
	return Coded{Status: 422, Err: err}
}

// UsesFallback returns a plain error: the default rule of Fallback gives it 422,
// so it is handled and not reported.
//
//effectgo:handler Fallback
func UsesFallback() error { // want UsesFallback:"."
	return Plain{}
}

// PassIs returns ErrPlain unchanged, and wraps every other error as a 422.
// errors.Is matches ErrPlain, so ErrPlain is a mapped variant, but the rule that
// matches it is a passthrough: it keeps the status of the variant, and ErrPlain
// has none. The first rule that matches a variant decides its status.
//
//effectgo:errmap
func PassIs(err error) error { // want PassIs:"." PassIs:"."
	if errors.Is(err, ErrPlain) {
		return err
	}
	return Coded{Status: 422, Err: err}
}

// IsUnhandled returns ErrPlain. It is mapped by errors.Is, but the passthrough
// keeps no status for it, so it is sent as 500 and reported.
//
//effectgo:handler PassIs
func IsUnhandled() error { // want IsUnhandled:"." "does not map"
	return ErrPlain
}

// IsDefault returns Plain, which no rule of PassIs matches: the default rule
// gives it 422, so it is handled.
//
//effectgo:handler PassIs
func IsDefault() error { // want IsDefault:"."
	return Plain{}
}

// ErrDynamic is a sentinel whose type has a StatusCode method. Its status is not
// known (it is the status of its value), so a passthrough that names it is silent.
var ErrDynamic = Dyn{Code: 503}

// PassDyn passes ErrDynamic through unchanged, and wraps every other error as a 422.
//
//effectgo:errmap
func PassDyn(err error) error { // want PassDyn:"." PassDyn:"."
	if errors.Is(err, ErrDynamic) {
		return err
	}
	return Coded{Status: 422, Err: err}
}

// IsDynamic returns ErrDynamic: not reported, its status is not known.
//
//effectgo:handler PassDyn
func IsDynamic() error { // want IsDynamic:"."
	return ErrDynamic
}
