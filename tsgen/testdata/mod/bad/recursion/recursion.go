// Package recursion has a type that refers to itself.
package recursion

import (
	"context"

	"github.com/faintghost/effectgo/rt/web"
)

// Node is a list that refers to itself.
type Node struct {
	Value string `json:"value"`
	Next  *Node  `json:"next"`
}

// Load returns a node.
func Load(ctx context.Context, in web.NoInput) (Node, error) { return Node{}, nil }

// Routes registers Load.
func Routes(r *web.Router) { web.Get(r, "/n", Load) }
