// Package ifaceuse uses and implements the contracted interfaces of ifacedef,
// and declares one of its own.
package ifaceuse

import (
	"database/sql"
	"net/http"
	"time"

	"ifacedef"
)

// SQLRepo implements ifacedef.Repo over database/sql. It conforms.
type SQLRepo struct{ DB *sql.DB }

func (s *SQLRepo) Get(id int) (string, error) { // want Get:`map\[db:`
	_ = s.DB.QueryRow("select 1")
	return "", nil
}

func (s *SQLRepo) Put(id int, v string) error { // want Put:`map\[db:`
	_, err := s.DB.Exec("insert")
	return err
}

// HTTPRepo implements ifacedef.Repo over HTTP. It violates the contract.
type HTTPRepo struct{ URL string }

func (h *HTTPRepo) Get(id int) (string, error) { // want Get:`map\[net:` `\(\*HTTPRepo\)\.Get violates contract of \(Repo\)\.Get \[allow db\]: has effect net via \(\*HTTPRepo\)\.Get → http\.Get`
	_, err := http.Get(h.URL)
	return "", err
}

func (h *HTTPRepo) Put(id int, v string) error { // want Put:`map\[\]`
	return nil
}

// fakeClock implements ifacedef.Clock. Now violates the type-level pure
// contract; Sleep conforms to its method-level allow time.
type fakeClock struct{}

func (fakeClock) Now() int { // want Now:`map\[time:` `fakeClock\.Now violates contract of \(Clock\)\.Now \[pure\]: has effect time via fakeClock\.Now → time\.Now`
	return int(time.Now().Unix())
}

func (fakeClock) Sleep(d int) { // want Sleep:`map\[time:`
	time.Sleep(time.Duration(d))
}

// Load uses the port under a pure contract: db is reported.
//
//effectgo:pure
func Load(r ifacedef.Repo, id int) (string, error) { // want Load:`map\[db:` `Load is declared //effectgo:pure but has effect db via Load → ifacedef\.\(Repo\)\.Get \[contract\]`
	return r.Get(id)
}

// LoadAllowed uses the same port with a contract that allows db.
//
//effectgo:allow db
func LoadAllowed(r ifacedef.Repo, id int) (string, error) { // want LoadAllowed:`map\[db:`
	return r.Get(id)
}

// TickOK reads the clock under its pure contract.
//
//effectgo:pure
func TickOK(c ifacedef.Clock) int { return c.Now() } // want TickOK:`map\[\]`

// Local is a contracted interface declared in this package.
//
//effectgo:pure
type Local interface {
	Ping() int // want Ping:`&\{\[\]\}`
}

type okLocal struct{}

func (okLocal) Ping() int { return 1 } // want Ping:`map\[\]`

type badLocal struct{}

func (badLocal) Ping() int { // want Ping:`map\[time:` `badLocal\.Ping violates contract of \(Local\)\.Ping \[pure\]: has effect time via badLocal\.Ping → time\.Now`
	return int(time.Now().Unix())
}
