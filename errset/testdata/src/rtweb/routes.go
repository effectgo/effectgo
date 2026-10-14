// Package rtweb registers handlers through the route functions of rtfake. Each
// route call is annotated with the report it must produce; a route without a
// marker must produce none.
package rtweb

import (
	"context"

	"rtfake"
	"rtlib"
	"rtshared"
)

// Routes registers the handlers.
func Routes(r *rtfake.Router, s *rtlib.Store) { // want Routes:"."
	rtfake.Get(r, "/ok", rtlib.GetOK)
	rtfake.Get(r, "/sentinel", rtlib.GetSentinel) // want "no constant StatusCode"
	rtfake.Get(r, "/fresh", rtlib.GetFresh)       // want "fresh error new@rtlib.GetFresh"
	rtfake.Get(r, "/dyn", rtlib.GetDyn)           // want "no constant StatusCode"
	rtfake.Get(r, "/plain", rtlib.GetPlain)       // want "no constant StatusCode"
	rtfake.Get(r, "/typed", rtlib.GetTyped)
	rtfake.Get(r, "/opaque", rtlib.GetOpaque)
	rtfake.Get(r, "/contract", rtlib.GetContract) // want "unclassified error"
	rtfake.Post(r, "/update", s.Update)           // want "no constant StatusCode"
	rtfake.Post(r, "/read", s.Read)
	rtfake.Get(r, "/foreign", rtshared.Find)                                              // want "rtshared.ErrMissing"
	rtfake.Get(r, "/closure", func(ctx context.Context, in rtlib.In) (rtlib.Out, error) { // want "function literal"
		return rtlib.Out{}, nil
	})
	rtfake.Get(r, "/variable", handlerVar)
}

var handlerVar func(context.Context, rtlib.In) (rtlib.Out, error) = rtlib.GetSentinel
