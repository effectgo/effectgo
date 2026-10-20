// Package catalog is a golden case: method handlers (the group is the receiver
// type), a path-only GET, a path-only DELETE with NoContent, and two errors with
// different statuses on one route. Its routes are registered by package app.
package catalog

import (
	"context"
	"net/http"

	"github.com/faintghost/effectgo/rt/web"
)

// GetIn is the path of GET and DELETE /api/items/{id}: no body, no query.
type GetIn struct {
	ID int64 `path:"id" json:"id" schema:"min=1"`
}

// Item is the response of GET /api/items/{id}.
type Item struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ItemNotFound is a 404.
type ItemNotFound struct {
	Detail string `json:"detail"`
}

func (ItemNotFound) Error() string   { return "item not found" }
func (ItemNotFound) StatusCode() int { return http.StatusNotFound }

// ItemLocked is a 409.
type ItemLocked struct {
	Detail string `json:"detail"`
}

func (ItemLocked) Error() string   { return "item locked" }
func (ItemLocked) StatusCode() int { return http.StatusConflict }

// Catalog holds the items.
type Catalog struct{}

// Get returns item 1.
func (c *Catalog) Get(_ context.Context, in GetIn) (Item, error) {
	if in.ID != 1 {
		return Item{}, ItemNotFound{Detail: "no such item"}
	}
	return Item{ID: in.ID, Name: "widget"}, nil
}

// Remove deletes item 1; item 2 is locked.
func (c *Catalog) Remove(_ context.Context, in GetIn) (web.NoContent, error) {
	switch in.ID {
	case 1:
		return web.NoContent{}, nil
	case 2:
		return web.NoContent{}, ItemLocked{Detail: "item is locked"}
	}
	return web.NoContent{}, ItemNotFound{Detail: "no such item"}
}
