package rtshape

import "reflect"

// RuleSpec is one rule of a schema tag. NoArg marks a rule written without an
// argument (trim, nonempty, ...); every other rule requires one. Cats are the
// rule categories the rule applies to (see KindCategory).
type RuleSpec struct {
	NoArg bool
	Cats  []string
}

// Rules is the closed vocabulary of schema tags (rt/schema parseRule).
var Rules = map[string]RuleSpec{
	"trim":       {NoArg: true, Cats: []string{"string"}},
	"singleline": {NoArg: true, Cats: []string{"string"}},
	"email":      {NoArg: true, Cats: []string{"string"}},
	"nonempty":   {NoArg: true, Cats: []string{"string", "slice", "map"}},
	"pattern":    {Cats: []string{"string"}},
	"min":        {Cats: []string{"int", "uint", "float", "string", "slice", "map"}},
	"max":        {Cats: []string{"int", "uint", "float", "string", "slice", "map"}},
}

// KindCategory maps the kinds that have a rule category to it (rt/schema
// categoryOf). A kind not listed has none: a bool, time.Time, json.RawMessage,
// a struct, an interface.
var KindCategory = map[reflect.Kind]string{
	reflect.String:  "string",
	reflect.Slice:   "slice",
	reflect.Map:     "map",
	reflect.Int:     "int",
	reflect.Int8:    "int",
	reflect.Int16:   "int",
	reflect.Int32:   "int",
	reflect.Int64:   "int",
	reflect.Uint:    "uint",
	reflect.Uint8:   "uint",
	reflect.Uint16:  "uint",
	reflect.Uint32:  "uint",
	reflect.Uint64:  "uint",
	reflect.Float32: "float",
	reflect.Float64: "float",
}

// SupportedKinds are the kinds rt/schema decodes (rt/schema checkType). Basic
// kinds outside this set (complex, uintptr, unsafe.Pointer), and chan, func and
// array, are not.
var SupportedKinds = []reflect.Kind{
	reflect.Bool, reflect.String,
	reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
	reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
	reflect.Float32, reflect.Float64,
	reflect.Pointer, reflect.Slice, reflect.Map, reflect.Struct, reflect.Interface,
}
