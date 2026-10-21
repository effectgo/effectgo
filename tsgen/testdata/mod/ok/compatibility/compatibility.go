package compatibility

import (
	"context"
	"encoding/json"

	"github.com/effectgo/effectgo/rt/web"
)

type Node struct {
	Value    string  `json:"value"`
	Next     *Node   `json:"next"`
	Children []*Node `json:"children"`
}
type Left struct {
	Right *Right `json:"right"`
}
type Right struct {
	Left *Left `json:"left"`
}

type Token struct{ Hidden chan int }

func (*Token) UnmarshalJSON(data []byte) error { var token string; return json.Unmarshal(data, &token) }

type TokenAlias = Token
type Decoders struct {
	Token  TokenAlias `json:"token"`
	Tokens []Token    `json:"tokens"`
}
type QuotedInt = int
type Quoted struct {
	Count   QuotedInt `json:"count,string"`
	Enabled bool      `json:"enabled,string"`
	Name    string    `json:"name,string"`
	Ratio   *float64  `json:"ratio,string"`
	Plain   []int     `json:"plain,string"`
}
type Output struct {
	OK bool `json:"ok"`
}

func Recursive(context.Context, Node) (Output, error)         { return Output{}, nil }
func Mutual(context.Context, Left) (Output, error)            { return Output{}, nil }
func RootDecoder(context.Context, TokenAlias) (Output, error) { return Output{}, nil }
func NestedDecoder(context.Context, Decoders) (Output, error) { return Output{}, nil }
func Quoting(context.Context, Quoted) (Output, error)         { return Output{}, nil }
func Exact(context.Context, Quoted) (Output, error)           { return Output{}, nil }
func Register(r *web.Router) {
	web.Post(r, "/recursive", Recursive, web.JSONBody())
	web.Post(r, "/mutual", Mutual, web.JSONBody())
	web.Post(r, "/root-decoder", RootDecoder, web.JSONBody())
	web.Post(r, "/nested-decoder", NestedDecoder, web.JSONBody())
	web.Post(r, "/quoting", Quoting, web.JSONBody())
	web.Post(r, "/exact", Exact, web.JSONBodyExact())
}
