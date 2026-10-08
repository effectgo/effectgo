// Command openapispec writes the OpenAPI document that Fuego generates for the
// todo server, as JSON. It is the input of effectgo openapi -check and -merge.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/faintghost/effectgo/examples/todo/api"
	"github.com/faintghost/effectgo/examples/todo/domain"
)

func main() {
	out := flag.String("o", "", "write the document to this file (default: stdout)")
	flag.Parse()
	s := api.NewServer(domain.NewService())
	data, err := json.MarshalIndent(s.OutputOpenAPISpec(), "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	data = append(data, '\n')
	if *out == "" {
		os.Stdout.Write(data)
		return
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatal(err)
	}
}
