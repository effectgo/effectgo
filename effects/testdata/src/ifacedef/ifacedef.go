// Package ifacedef declares contracted interfaces. Implementations and callers
// live in package ifaceuse.
package ifacedef

// Repo is the persistence port. Its methods may touch the database only.
//
//effectgo:allow db
type Repo interface {
	Get(id int) (string, error) // want Get:`&\{\[db\]\}`
	Put(id int, v string) error // want Put:`&\{\[db\]\}`
}

// Clock is pure by default. Sleep overrides the type-level contract.
//
//effectgo:pure
type Clock interface {
	Now() int // want Now:`&\{\[\]\}`
	//effectgo:allow time
	Sleep(d int) // want Sleep:`&\{\[time\]\}`
}

// Store has no contract, so its calls stay unknown.
type Store interface {
	Load(key string) string
}

// Tick reads the clock through its contract. Pure.
//
//effectgo:pure
func Tick(c Clock) int { return c.Now() } // want Tick:`map\[\]`

// Nap calls Sleep, which the Clock contract allows only as time.
//
//effectgo:pure
func Nap(c Clock) { c.Sleep(1) } // want Nap:`map\[time:` `Nap is declared //effectgo:pure but has effect time via Nap → \(Clock\)\.Sleep \[contract\]`

// UseStore calls an unannotated interface method: unknown.
func UseStore(s Store) string { return s.Load("k") } // want UseStore:`map\[unknown:`
