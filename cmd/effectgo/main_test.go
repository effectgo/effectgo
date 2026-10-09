package main

import (
	"reflect"
	"testing"
)

func TestWithoutTests(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"standalone defaults to no tests", []string{"effectgo", "./..."}, []string{"effectgo", "-test=false", "./..."}},
		{"flags before patterns are kept in order", []string{"effectgo", "-errset.report", "./..."}, []string{"effectgo", "-test=false", "-errset.report", "./..."}},
		{"-test=true is kept", []string{"effectgo", "-test=true", "./..."}, []string{"effectgo", "-test=true", "./..."}},
		{"-test alone is kept", []string{"effectgo", "-test", "./..."}, []string{"effectgo", "-test", "./..."}},
		{"--test=true is kept", []string{"effectgo", "--test=true", "./..."}, []string{"effectgo", "--test=true", "./..."}},
		{"go vet -flags is left alone", []string{"effectgo", "-flags"}, []string{"effectgo", "-flags"}},
		{"go vet -V=full is left alone", []string{"effectgo", "-V=full"}, []string{"effectgo", "-V=full"}},
		{"go vet config file is left alone", []string{"effectgo", "-stubs=x", "/tmp/vet.cfg"}, []string{"effectgo", "-stubs=x", "/tmp/vet.cfg"}},
		{"no arguments", []string{"effectgo"}, []string{"effectgo"}},
	}
	for _, c := range cases {
		if got := withoutTests(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: withoutTests(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
