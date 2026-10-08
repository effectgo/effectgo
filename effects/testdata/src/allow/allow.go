package allow

import (
	"database/sql"
	"time"
)

//effectgo:allow time,db
func OK(db *sql.DB) error { // want OK:`map\[db:.*time:`
	time.Sleep(0)
	return db.Ping()
}

//effectgo:allow time
func Bad(db *sql.DB) error { // want Bad:`map\[db:` `Bad is declared //effectgo:allow time but has effect db via Bad → sql\.\(\*DB\)\.Ping`
	return db.Ping()
}

//effectgo:allow tiem // want `unknown effect name .tiem.`
func Typo() {} // want Typo:`map\[\]`

//effectgo:purr // want `unknown effectgo directive .purr.`
func Typo2() {} // want Typo2:`map\[\]`
