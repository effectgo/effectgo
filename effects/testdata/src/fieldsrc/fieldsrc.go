// Package fieldsrc exercises field source rules (effects field:(pkg.T).F): the
// body of an http.Request or http.Response is network data, so a read through
// it is net. The field is read directly, through a single-assignment local, or
// through the reader that a constructor stores. Another field, or a local that
// is reassigned or whose address is taken, stays unknown.
package fieldsrc

import (
	"encoding/json"
	"io"
	"net/http"
)

// ReadAll reads the request body directly: io.ReadAll calls Read on it.
//
//effectgo:pure
func ReadAll(r *http.Request) ([]byte, error) { // want ReadAll:`net` `ReadAll is declared //effectgo:pure but has effect net via ReadAll → io\.ReadAll → http\.\(Request\)\.Body \[field\]`
	return io.ReadAll(r.Body)
}

// DecodeDirect decodes the request body through a decoder built on the field.
//
//effectgo:pure
func DecodeDirect(r *http.Request, v any) error { // want DecodeDirect:`net` `DecodeDirect is declared //effectgo:pure but has effect net`
	return json.NewDecoder(r.Body).Decode(v)
}

// DecodeLocal decodes through a local variable that holds the field.
//
//effectgo:pure
func DecodeLocal(r *http.Request, v any) error { // want DecodeLocal:`net` `DecodeLocal is declared //effectgo:pure but has effect net`
	body := r.Body
	return json.NewDecoder(body).Decode(v)
}

// DecodeStored decodes through a decoder kept in a local, the stored value being
// the field.
//
//effectgo:pure
func DecodeStored(r *http.Request, v any) error { // want DecodeStored:`net` `DecodeStored is declared //effectgo:pure but has effect net`
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// DecodeStoredLocal stores through a local that is itself a single-assignment
// field load.
//
//effectgo:pure
func DecodeStoredLocal(r *http.Request, v any) error { // want DecodeStoredLocal:`net` `DecodeStoredLocal is declared //effectgo:pure but has effect net`
	body := r.Body
	dec := json.NewDecoder(body)
	return dec.Decode(v)
}

// DecodeResponse reads a response stream: net/http.Response.Body is net.
//
//effectgo:pure
func DecodeResponse(resp *http.Response, v any) error { // want DecodeResponse:`net` `DecodeResponse is declared //effectgo:pure but has effect net`
	return json.NewDecoder(resp.Body).Decode(v)
}

// Wrapped embeds a request: the body is promoted from the embedded Request, so
// it is the same field.
type Wrapped struct {
	*http.Request
}

// DecodePromoted reads the promoted body.
//
//effectgo:pure
func DecodePromoted(w Wrapped, v any) error { // want DecodePromoted:`net` `DecodePromoted is declared //effectgo:pure but has effect net`
	return json.NewDecoder(w.Body).Decode(v)
}

// Other has a field named Body of another type: it is not a field source.
type Other struct {
	Body io.ReadCloser
}

// DecodeOther reads the field of another type: unknown.
//
//effectgo:pure
func DecodeOther(o *Other, v any) error { // want DecodeOther:`unknown` `DecodeOther is declared //effectgo:pure but has effect unknown`
	return json.NewDecoder(o.Body).Decode(v)
}

// DecodeReassigned reads through a local with two assignments: unknown.
//
//effectgo:pure
func DecodeReassigned(r *http.Request, o *Other, v any) error { // want DecodeReassigned:`unknown` `DecodeReassigned is declared //effectgo:pure but has effect unknown`
	body := r.Body
	body = o.Body
	return json.NewDecoder(body).Decode(v)
}

// DecodeAddr reads through a local whose address is taken: unknown.
//
//effectgo:pure
func DecodeAddr(r *http.Request, v any) error { // want DecodeAddr:`unknown` `DecodeAddr is declared //effectgo:pure but has effect unknown`
	body := r.Body
	p := &body
	_ = p
	return json.NewDecoder(body).Decode(v)
}

// DecodeAllowed declares the contract that the field source makes true.
//
//effectgo:allow net
func DecodeAllowed(r *http.Request, v any) error { // want DecodeAllowed:`map\[net:`
	return json.NewDecoder(r.Body).Decode(v)
}
