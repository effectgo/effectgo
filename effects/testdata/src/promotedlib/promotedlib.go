// Package promotedlib has a sink whose Write reads the clock.
package promotedlib

import "time"

// Sink writes to the clock.
type Sink struct{}

// Write reads the clock.
func (*Sink) Write(p []byte) (int, error) {
	_ = time.Now()
	return len(p), nil
}
