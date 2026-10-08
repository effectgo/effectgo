// Package repo is the persistence port of the demo, with two adapters: one
// over database/sql (conforms to the contract) and one over HTTP (violates it).
package repo

import (
	"database/sql"
	"net/http"
)

// Repo is the persistence port. Implementations may touch the database only.
//
//effectgo:allow db
type Repo interface {
	Get(id int) (string, error)
}

// SQLRepo implements Repo over database/sql. Its effects are db only.
type SQLRepo struct{ DB *sql.DB }

// Get loads a title by id.
func (r *SQLRepo) Get(id int) (string, error) {
	var title string
	err := r.DB.QueryRow("select title from todos where id = ?", id).Scan(&title)
	return title, err
}

// HTTPRepo implements Repo over HTTP. Its Get reaches the network, which the
// Repo contract does not allow: this is a violation.
type HTTPRepo struct{ URL string }

// Get asks the remote service for the status of an item.
func (r *HTTPRepo) Get(id int) (string, error) {
	resp, err := http.Get(r.URL)
	if err != nil {
		return "", err
	}
	return resp.Status, nil
}
