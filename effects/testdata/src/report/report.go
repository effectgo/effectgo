package report

import (
	"os"
	"time"
)

func Pure() int { return 1 } // want Pure:`map\[\]` `effects of Pure: \{\}`

func Clock() time.Time { return time.Now() } // want Clock:`map\[time:` `effects of Clock: \{time\}`

func Both() string { // want Both:`map\[env:.*time:` `effects of Both: \{env, time\}`
	_ = time.Now()
	return os.Getenv("HOME")
}

// Each calls fn on every element; its only effect is the call of its parameter.
func Each(xs []int, fn func(int)) { // want Each:`map\[param:1:` `effects of Each: \{param:1\}`
	for _, x := range xs {
		fn(x)
	}
}
