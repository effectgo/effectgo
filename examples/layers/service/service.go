// Package service orchestrates infra for the application.
package service

import (
	"fmt"
	"os"

	"github.com/faintghost/effectgo/examples/layers/infra"
)

// Sync fetches a URL and prints the status. Declared to use net and stdio only.
//
//effectgo:allow net,stdio
func Sync(url string) error {
	code, err := infra.Fetch(url)
	if err != nil {
		return err
	}
	fmt.Println("status", code)
	return nil
}

// Mode reads configuration from the environment; no contract.
func Mode() string {
	return os.Getenv("SERVICE_MODE")
}

// Log prints a message. Declared to use stdio only.
//
//effectgo:allow stdio
func Log(msg string) {
	fmt.Println(msg)
}
