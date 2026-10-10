package main

import "testing"

func TestRelPos(t *testing.T) {
	dir := "/work/app"
	cases := map[string]string{
		"/work/app/api/orders.go:10:3": "api/orders.go:10:3",
		"/work/app/a.go:1":             "a.go:1",
		"/elsewhere/b.go:2:1":          "/elsewhere/b.go:2:1",
		"":                             "",
	}
	for in, want := range cases {
		if got := relPos(in, dir); got != want {
			t.Errorf("relPos(%q) = %q, want %q", in, got, want)
		}
	}
}
