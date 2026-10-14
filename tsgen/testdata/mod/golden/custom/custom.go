// Package custom is a golden case: types with their own JSON methods. Stats has
// unexported fields and a pointer-receiver MarshalJSON, as store.Stats does, so
// its response is Schema.Unknown. Bulk decodes its own request JSON, so the request
// is Schema.Unknown. Code is an enum whose response is its MarshalText string.
// CustomError encodes its own body. The Go fields say nothing about these shapes.
package custom

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/faintghost/effectgo/rt/web"
)

// Stats counts items; its JSON object has the keys all and sent.
type Stats struct {
	all  int
	sent int
}

// MarshalJSON writes the object, not the fields.
func (s *Stats) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int{"all": s.all, "sent": s.sent})
}

// Code is an enum whose JSON form is its upper-case text.
type Code string

const (
	CodeA Code = "a"
	CodeB Code = "b"
)

// Valid reports whether c is a code.
func (c Code) Valid() bool { return c == CodeA || c == CodeB }

// MarshalText writes the upper-case code.
func (c Code) MarshalText() ([]byte, error) { return []byte(strings.ToUpper(string(c))), nil }

// Bulk is read from the request by its own decoder: a JSON array of numbers.
type Bulk struct {
	Items []int `json:"items"`
}

// UnmarshalJSON reads the array.
func (b *Bulk) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &b.Items)
}

// CustomIn is the body of POST /api/custom.
type CustomIn struct {
	Bulk Bulk `json:"bulk"`
	Code Code `json:"code"`
}

// CustomOut is the response of POST /api/custom.
type CustomOut struct {
	Stats *Stats `json:"stats"`
	Code  Code   `json:"code"`
}

// CustomError is a 422 whose body is its own encoding.
type CustomError struct {
	Reason string `json:"reason"`
}

// Error describes the error.
func (CustomError) Error() string { return "custom error" }

// StatusCode reports 422.
func (CustomError) StatusCode() int { return http.StatusUnprocessableEntity }

// MarshalJSON writes the reason as a bare string.
func (e CustomError) MarshalJSON() ([]byte, error) { return json.Marshal(e.Reason) }

// Custom answers with the stats of the bulk.
func Custom(_ context.Context, in CustomIn) (CustomOut, error) {
	if len(in.Bulk.Items) == 0 {
		return CustomOut{}, CustomError{Reason: "empty"}
	}
	return CustomOut{Stats: &Stats{all: len(in.Bulk.Items)}, Code: in.Code}, nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Post(r, "/api/custom", Custom)
}
