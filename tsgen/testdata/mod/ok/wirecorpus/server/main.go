package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"example.com/tsbad/ok/wirecorpus"
	"github.com/faintghost/effectgo/rt/web"
)

func main() {
	var corpus []struct {
		Name    string          `json:"name"`
		Payload json.RawMessage `json:"payload"`
	}
	data, err := os.ReadFile("ok/wirecorpus/cases.json")
	if err != nil || json.Unmarshal(data, &corpus) != nil || len(corpus) == 0 {
		panic("invalid wire corpus")
	}
	r := web.New()
	wirecorpus.Register(r)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	srv := &http.Server{Handler: r}
	fmt.Println("READY", "http://"+ln.Addr().String(), len(corpus))
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
