// Package dirparse reads the keyword and the arguments of an //effectgo: comment.
// Every analyzer that reads a directive uses this rule, so that they agree on the
// keyword of a comment: the words after the prefix, up to the first word that
// starts with //. A trailing comment after a directive, as in
// "//effectgo:mustuse // handled by the caller", is therefore not an argument.
package dirparse

import "strings"

// Prefix starts every directive comment.
const Prefix = "//effectgo:"

// Words returns the words of a directive body (the text after Prefix), up to the
// first word that starts with //.
func Words(body string) []string {
	var out []string
	for _, f := range strings.Fields(body) {
		if strings.HasPrefix(f, "//") {
			break
		}
		out = append(out, f)
	}
	return out
}

// Parse returns the keyword and the arguments of a comment. ok is false when the
// comment is not a directive. The keyword is empty when no word follows the prefix.
func Parse(text string) (kw string, args []string, ok bool) {
	body, ok := strings.CutPrefix(text, Prefix)
	if !ok {
		return "", nil, false
	}
	words := Words(body)
	if len(words) == 0 {
		return "", nil, true
	}
	return words[0], words[1:], true
}
