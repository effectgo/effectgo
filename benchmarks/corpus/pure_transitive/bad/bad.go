package bad

import (
	"time"

	"example.com/effectgo-bench/pure_transitive/clock"
)

//effectgo:pure
func Stamp() time.Time {
	return clock.Now()
}
