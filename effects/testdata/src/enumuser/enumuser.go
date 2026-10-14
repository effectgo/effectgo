// Package enumuser implements enumdef.Enum and imports it. Its Valid reads the
// clock, which the contract forbids: the conformance check reports it here.
package enumuser

import (
	"time"

	"enumdef"
)

// Seg is an enum whose Valid breaks the pure contract of enumdef.Enum.
type Seg string

func (s Seg) Valid() bool { // want Valid:`map\[time:` `Seg\.Valid violates contract of \(Enum\)\.Valid \[pure\]: has effect time via Seg\.Valid → time\.Now`
	return time.Now().Unix() > 0 && s != ""
}

// Pure is an enum whose Valid is pure: no finding.
type Pure string

func (p Pure) Valid() bool { return p != "" } // want Valid:`map\[\]`

var _ enumdef.Enum = Seg("")
