package infra

import (
	"net/http"
	"time"
)

func Fetch(url string) error {
	_, err := http.Get(url)
	return err
}

func Now() time.Time { return time.Now() }

func Wrap() error { return Fetch("http://example.com") }
