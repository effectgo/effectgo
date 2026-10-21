// Package scalars is a golden case: the scalar kinds in both directions, and the
// 64-bit comment on every integer kind that is 64 bits wide. Its handler is a
// plain function, so its group is the package name.
package scalars

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

// ScalarsIn is the request body of POST /api/scalars.
type ScalarsIn struct {
	S   string  `json:"s"`
	I   int     `json:"i"`
	I32 int32   `json:"i32"`
	I64 int64   `json:"i64"`
	U   uint    `json:"u"`
	U64 uint64  `json:"u64"`
	F   float64 `json:"f"`
	B   bool    `json:"b"`
}

// ScalarsOut is the response of POST /api/scalars.
type ScalarsOut struct {
	S   string  `json:"s"`
	I   int     `json:"i"`
	I32 int32   `json:"i32"`
	I64 int64   `json:"i64"`
	U   uint    `json:"u"`
	U64 uint64  `json:"u64"`
	F   float64 `json:"f"`
	B   bool    `json:"b"`
}

// Scalars echoes the input.
func Scalars(_ context.Context, in ScalarsIn) (ScalarsOut, error) {
	return ScalarsOut(in), nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Post(r, "/api/scalars", Scalars)
}
