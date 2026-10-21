// Package tagbad has schema tags that rt/schema rejects: unknown and repeated
// rules, malformed arguments, and rules that do not fit the field type. A pattern
// name is not checked against the registry (see docs/reference/analyzers.md).
package tagbad

import "github.com/effectgo/effectgo/rt/schema"

// Bad is decoded with schema.Decode, so every tag is read.
type Bad struct {
	Name    string  `json:"name" schema:"trim,bogus"` // want `unknown rule "bogus"`
	Max     int     `json:"max" schema:"max=abc"`     // want `"abc" is not an integer`
	Dup     string  `json:"dup" schema:"trim,trim"`   // want `rule "trim" is repeated`
	Email   int     `json:"email" schema:"email"`     // want `rule "email" does not apply to int`
	Flag    bool    `json:"flag" schema:"min=1"`      // want `rule "min" does not apply to bool`
	Count   int     `json:"count" schema:"nonempty"`  // want `rule "nonempty" does not apply to int`
	Opt     string  `json:"opt" schema:"trim=1"`      // want `rule "trim" takes no argument`
	Limit   int     `json:"limit" schema:"min"`       // want `rule "min" needs a value`
	Slug    string  `json:"slug" schema:"pattern=nope"`
	Pat     int     `json:"pat" schema:"pattern=nope"`            // want `rule "pattern" does not apply to int`
	Ptr     *int    `json:"ptr" schema:"min=x"`                   // want `"x" is not an integer`
	Empty   string  `json:"empty" schema:"trim,"`                 // want `empty rule`
	Neg     string  `json:"neg" schema:"max=-1"`                  // want `"-1" is not a length`
	NaN     float64 `json:"nan" schema:"max=NaN"`                 // want `"NaN" is not a number`
	Uint    uint    `json:"uint" schema:"min=-3"`                 // want `"-3" is not an unsigned integer`
	Opt2    *bool   `json:"opt2" schema:"email"`                  // want `rule "email" does not apply to bool`
	NoValue string  `json:"novalue" schema:"pattern"`             // want `rule "pattern" needs a pattern name`
	Unknown []int   `json:"unknown" schema:"nonempty,singleline"` // want `rule "singleline" does not apply to \[\]int`
}

// Decode reads Bad.
func Decode(data []byte) (Bad, error) {
	return schema.Decode[Bad](data)
}
