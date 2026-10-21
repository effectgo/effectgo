package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/effectgo/effectgo/rt/web"
)

type SupportedError struct {
	Detail string  `json:"detail" schema:"nonempty"`
	Count  int     `json:"count,string"`
	Note   *string `json:"note,omitempty"`
}

func (SupportedError) Error() string   { return "external error" }
func (SupportedError) StatusCode() int { return http.StatusUnauthorized }

func Read(context.Context, web.NoInput) (web.NoContent, error) {
	return web.NoContent{}, SupportedError{Count: 7}
}

func Register(r *web.Router) {
	web.Get(r, "/external-wire", Read, web.ExternalError[SupportedError]())
}

func main() {
	r := web.New(web.WithErrorEncoder(func(w http.ResponseWriter, _ *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(err)
	}))
	Register(r)
	server := httptest.NewServer(r)
	defer server.Close()
	response, err := http.Get(server.URL + "/external-wire")
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	_ = json.NewEncoder(os.Stdout).Encode(struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}{response.StatusCode, body})
}
