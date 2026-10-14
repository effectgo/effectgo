// Package enumbad has sealed enums that schema cannot limit: no Valid method,
// and a Valid with the wrong signature.
package enumbad

import "github.com/faintghost/effectgo/rt/schema"

// Kind is sealed and has no Valid.
//
//effectgo:sealed
type Kind string // want Kind:"sealed enum"

const (
	KindA Kind = "a"
	KindB Kind = "b"
)

// Mode is sealed and its Valid returns an int, so it is not the Enum method.
//
//effectgo:sealed
type Mode int // want Mode:"sealed enum"

const ModeOn Mode = 1

// Valid has the wrong signature.
func (m Mode) Valid() int { return int(m) }

// Holder is decoded, so its enum fields are checked.
type Holder struct {
	K    Kind   `json:"k"`    // want `field Holder.K: type Kind is a sealed enum with no Valid\(\) bool method`
	M    Mode   `json:"m"`    // want `field Holder.M: type Mode is a sealed enum with no Valid\(\) bool method`
	Many []Kind `json:"many"` // want `field Holder.Many: type Kind is a sealed enum`
}

// Decode reads a Holder.
func Decode(data []byte) (Holder, error) {
	return schema.Decode[Holder](data)
}
