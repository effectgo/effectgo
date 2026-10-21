// Command server runs the todo API with /v1 (idiomatic) and /v2 (Result)
// routes on one Fuego server.
package main

import (
	"flag"
	"log"

	"github.com/effectgo/effectgo/examples/todo/api"
	"github.com/effectgo/effectgo/examples/todo/domain"
	"github.com/go-fuego/fuego"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()
	svc := domain.NewService()
	s := api.NewServer(svc, fuego.WithAddr(*addr))
	log.Fatal(s.Run())
}
