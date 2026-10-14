// Command web is the M0 spike of the effectgo framework: two hand-written
// endpoints whose errors are consumed by the TypeScript client in ./ts.
//
// PORT selects the listen port (default 8080; 0 picks a free port and the
// chosen address is printed).
package main

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("listening on %s\n", ln.Addr())
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	log.Fatal(http.Serve(ln, NewHandler(NewStore(), logger)))
}
