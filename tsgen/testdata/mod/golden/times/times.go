// Package times is a golden case: time.Time, json.RawMessage and any, in both
// directions.
package times

import (
	"context"
	"encoding/json"
	"time"

	"github.com/faintghost/effectgo/rt/web"
)

// TimesIn is the request body of POST /api/times.
type TimesIn struct {
	At  time.Time       `json:"at"`
	Raw json.RawMessage `json:"raw"`
	Any any             `json:"any"`
}

// TimesOut is the response of POST /api/times.
type TimesOut struct {
	At  time.Time       `json:"at"`
	Raw json.RawMessage `json:"raw"`
	Any any             `json:"any"`
}

// Times echoes the input.
func Times(_ context.Context, in TimesIn) (TimesOut, error) {
	return TimesOut(in), nil
}

// Register adds the route.
func Register(r *web.Router) {
	web.Post(r, "/api/times", Times)
}
