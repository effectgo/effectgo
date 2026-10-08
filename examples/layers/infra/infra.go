// Package infra talks to the outside world.
package infra

import (
	"net/http"
	"time"
)

// Fetch performs a network request and reports the status code.
func Fetch(url string) (int, error) {
	resp, err := http.Head(url)
	if err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}

// Stamp returns the current time as text.
func Stamp() string {
	return time.Now().Format(time.RFC3339)
}
