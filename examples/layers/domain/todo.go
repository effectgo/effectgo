package domain

import (
	"strings"

	"github.com/faintghost/effectgo/examples/layers/infra"
	"github.com/faintghost/effectgo/examples/layers/repo"
)

// Todo is a single item.
type Todo struct {
	Title string
	Done  bool
}

// Sum adds two counts.
func Sum(a, b int) int { return a + b }

// Describe renders a todo for display.
func Describe(t Todo) string {
	if t.Done {
		return "[x] " + t.Title
	}
	return "[ ] " + t.Title
}

// Normalize trims a title and appends a stamp. Declared pure, but the stamp
// reaches time.Now through infra.Stamp: a two-hop leak.
//
//effectgo:pure
func Normalize(s string) string {
	return strings.TrimSpace(s) + infra.Stamp()
}

// Save persists a todo. Layering violation: domain must not reach the network.
func Save(t Todo, url string) error {
	_, err := infra.Fetch(url)
	return err
}

// Load fetches a title through the persistence port. The package default is
// pure, and db is not pure, so this is reported: a pure domain function may not
// use a port whose contract allows db.
func Load(r repo.Repo, id int) (string, error) {
	return r.Get(id)
}

// LoadAllowed is the same call with its own contract, which allows db. Not reported.
//
//effectgo:allow db
func LoadAllowed(r repo.Repo, id int) (string, error) {
	return r.Get(id)
}

// ErrText renders an error. err.Error is an interface call; with the shipped
// stubs it is pure (no false positive), without them it is unknown.
func ErrText(err error) string {
	return "error: " + err.Error()
}
