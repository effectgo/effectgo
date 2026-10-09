// Package httpw checks the net/http rules of the shipped stub file.
package httpw

import (
	"net/http"
)

// Header reads the header map, which is pure.
//
//effectgo:pure
func Header(w http.ResponseWriter) http.Header { return w.Header() } // want Header:`map\[\]`

// Write sends the body: net.
//
//effectgo:pure
func Write(w http.ResponseWriter) { // want Write:`map\[net:` `Write is declared //effectgo:pure but has effect net via Write → http\.\(ResponseWriter\)\.Write \[stub\]`
	_, _ = w.Write(nil)
}

// Status sends the status: net.
//
//effectgo:pure
func Status(w http.ResponseWriter) { // want Status:`map\[net:` `Status is declared //effectgo:pure but has effect net via Status → http\.\(ResponseWriter\)\.WriteHeader \[stub\]`
	w.WriteHeader(http.StatusTeapot)
}

// Fail uses http.Error, Redirect and NotFound: net.
//
//effectgo:pure
func Fail(w http.ResponseWriter, r *http.Request) { // want Fail:`map\[net:` `Fail is declared //effectgo:pure but has effect net via Fail → http\.Error`
	http.Error(w, "x", http.StatusBadRequest)
	http.Redirect(w, r, "/", http.StatusFound)
	http.NotFound(w, r)
}

// Serve opens a file: fs and net.
//
//effectgo:pure
func Serve(w http.ResponseWriter, r *http.Request) { // want Serve:`map\[fs:` `Serve is declared //effectgo:pure but has effect fs via Serve → http\.ServeFile` `Serve is declared //effectgo:pure but has effect net via Serve → http\.ServeFile`
	http.ServeFile(w, r, "x.txt")
}
