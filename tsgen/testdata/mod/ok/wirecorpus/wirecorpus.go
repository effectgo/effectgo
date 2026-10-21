package wirecorpus

import (
	"context"

	"github.com/effectgo/effectgo/rt/web"
)

type Nested struct {
	Optional *string `json:"optional,omitempty"`
	Flag     bool    `json:"flag"`
	Count    int     `json:"count"`
}

type OutputNested Nested

type Input struct {
	Name     string             `json:"name"`
	Enabled  bool               `json:"enabled"`
	Count    int                `json:"count"`
	Optional *string            `json:"optional,omitempty"`
	Nested   *Nested            `json:"nested,omitempty"`
	Items    []Nested           `json:"items"`
	Lookup   map[string]*Nested `json:"lookup"`
}

type Output struct {
	Value    string                   `json:"value"`
	Optional *string                  `json:"optional,omitempty"`
	Null     *string                  `json:"null"`
	Nested   *OutputNested            `json:"nested,omitempty"`
	Items    []OutputNested           `json:"items,omitempty"`
	Lookup   map[string]*OutputNested `json:"lookup"`
}

func Echo(_ context.Context, in Input) (Output, error) {
	items := make([]OutputNested, len(in.Items))
	for i, item := range in.Items {
		items[i] = OutputNested(item)
	}
	lookup := make(map[string]*OutputNested, len(in.Lookup))
	for key, item := range in.Lookup {
		lookup[key] = (*OutputNested)(item)
	}
	return Output{Value: in.Name, Optional: in.Optional, Null: in.Optional, Nested: (*OutputNested)(in.Nested), Items: items, Lookup: lookup}, nil
}

func Register(r *web.Router) {
	web.Post(r, "/wire", Echo)
}
