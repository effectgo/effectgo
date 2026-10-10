package dirparse

import (
	"slices"
	"testing"
)

func TestWords(t *testing.T) {
	cases := []struct {
		body string
		want []string
	}{
		{"", nil},
		{"pure", []string{"pure"}},
		{" pure ", []string{"pure"}},
		{"pure // handled by the caller", []string{"pure"}},
		{"allow db, time // why", []string{"allow", "db,", "time"}},
		{"sealed extra", []string{"sealed", "extra"}},
		{"sealed extra // why", []string{"sealed", "extra"}},
		{"mustuse//note", []string{"mustuse//note"}},
		{"mustuse\t//note", []string{"mustuse"}},
		{"//note", nil},
	}
	for _, c := range cases {
		if got := Words(c.body); !slices.Equal(got, c.want) {
			t.Errorf("Words(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		text string
		kw   string
		args []string
		ok   bool
	}{
		{"// plain comment", "", nil, false},
		{"//effectgo:", "", nil, true},
		{"//effectgo:pure", "pure", nil, true},
		{"//effectgo:pure // a trailing comment", "pure", nil, true},
		{"//effectgo:mustuse extra", "mustuse", []string{"extra"}, true},
		{"//effectgo:errors A, B", "errors", []string{"A,", "B"}, true},
		{"// effectgo:pure", "", nil, false},
	}
	for _, c := range cases {
		kw, args, ok := Parse(c.text)
		if kw != c.kw || ok != c.ok || !slices.Equal(args, c.args) {
			t.Errorf("Parse(%q) = (%q, %q, %v), want (%q, %q, %v)", c.text, kw, args, ok, c.kw, c.args, c.ok)
		}
	}
}
