package domain

import "infra"

func Sum(a, b int) int { return a + b } // want Sum:`map\[\]`

func Load() error { // want Load:`map\[net:\[\{domain domain Load\} \{infra infra Fetch\} \{net/http http Get\}\]\]` `Load is declared //effectgo:package pure but has effect net via Load → infra\.Fetch → http\.Get`
	return infra.Fetch("http://example.com")
}

func Stamp() string { // want Stamp:`map\[time:\[\{domain domain Stamp\} \{infra infra Now\} \{time time Now\}\]\]` `Stamp is declared //effectgo:package pure but has effect time via Stamp → infra\.Now → time\.Now`
	return infra.Now().String()
}

// Fetcher overrides the package default, so the network use is allowed.
//
//effectgo:allow net
func Fetcher() error { // want Fetcher:`map\[net:`
	return infra.Fetch("http://example.com")
}
