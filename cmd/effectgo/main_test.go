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

func TestNoStubsWarning(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  string
		warn bool
	}{
		{"standalone, nothing configured", []string{"effectgo", "./..."}, "", true},
		{"sig, nothing configured", []string{"effectgo", "sig", "./..."}, "", true},
		{"openapi, nothing configured", []string{"effectgo", "openapi", "./..."}, "", true},
		{"whitespace env is not configured", []string{"effectgo", "./..."}, "  ", true},
		{"-stubs=file", []string{"effectgo", "-stubs=/a.stubs", "./..."}, "", false},
		{"--stubs=file", []string{"effectgo", "--stubs=/a.stubs", "./..."}, "", false},
		{"-stubs file (separate argument)", []string{"effectgo", "-stubs", "/a.stubs", "./..."}, "", false},
		{"openapi with -stubs after the subcommand", []string{"effectgo", "openapi", "-stubs=/a.stubs", "./..."}, "", false},
		{"-stubs= is empty, so not configured", []string{"effectgo", "-stubs=", "./..."}, "", true},
		{"EFFECTGO_STUBS set", []string{"effectgo", "./..."}, "/a.stubs,/b.stubs", false},
		{"go vet -flags probe is quiet", []string{"effectgo", "-flags"}, "", false},
		{"go vet -V=full probe is quiet", []string{"effectgo", "-V=full"}, "", false},
		{"go vet config file is quiet", []string{"effectgo", "/tmp/vet.cfg"}, "", false},
	}
	for _, c := range cases {
		got := noStubsWarning(c.args, c.env)
		if c.warn && got != noStubsWarningText {
			t.Errorf("%s: no warning, want %q", c.name, noStubsWarningText)
		}
		if !c.warn && got != "" {
			t.Errorf("%s: unexpected warning %q", c.name, got)
		}
	}
}
