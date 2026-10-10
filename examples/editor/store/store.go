// Package store is a persistence stand-in. Its body does no I/O; the project
// stub file declares that Save has the db effect.
package store

// Save stores a value.
func Save(v string) error {
	_ = v
	return nil
}
