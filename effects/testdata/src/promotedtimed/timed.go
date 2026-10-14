// Package promotedtimed embeds a contracted interface, whose contract allows time, in
// a type that must satisfy promoted.Writer (allow stdio). The embedded interface is
// in this package, so its contract binds only the types of this package: the
// violation is the method promoted from it, reported at the embedding field.
package promotedtimed

import "promoted"

// timed reads the clock, as its contract says.
//
//effectgo:allow time
type timed interface {
	Write(p []byte) (int, error) // want Write:`&\{\[time\]\}`
}

// timedOuter promotes Write from timed: the contract of timed allows time, which the
// contract of promoted.Writer does not.
type timedOuter struct {
	timed // want `\(\*timedOuter\)\.Write violates contract of \(Writer\)\.Write \[allow stdio\]: has effect time via \(\*timedOuter\)\.Write \[promoted from embedded promotedtimed\.timed\] → \(timed\)\.Write \[contract\]`
}

// NewTimedOuter returns timedOuter as a promoted.Writer.
func NewTimedOuter(t timed) promoted.Writer { return &timedOuter{timed: t} } // want NewTimedOuter:`map\[\]`
