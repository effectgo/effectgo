// Package handlers is the analysis fixture: a sink mapper, a status mapper with
// its fallthrough, inline branches, leaks, a fresh error, and the routes.
package handlers

import (
	"errors"

	"github.com/gin-gonic/gin"

	"fixture/httpapi"
	"fixture/store"
)

// storeError is a sink mapper: two variant branches and a 500 default.
func storeError(c *gin.Context, err error) { // want storeError:"gin-sink-mapper"
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpapi.Detail(c, 404, "missing")
	case errors.Is(err, store.ErrConflict):
		httpapi.Detail(c, 409, "conflict")
	default:
		httpapi.Detail(c, 500, "db error")
	}
}

// statusOf is a status mapper (the (int, bool) shape).
func statusOf(err error) (int, bool) { // want statusOf:"gin-status-mapper"
	switch {
	case errors.Is(err, store.ErrInvalid):
		return 400, true
	}
	return 0, false
}

// GetItem calls the sink mapper: ErrInvalid is not a branch, so the default 500
// reaches it.
func GetItem(s *store.Store) gin.HandlerFunc { // want GetItem:"gin-handler-factory"
	return func(c *gin.Context) {
		_, err := s.Get(1)
		if err != nil {
			storeError(c, err)
			return
		}
		c.JSON(200, gin.H{"ok": true})
	}
}

// Create answers an inline error with 500 and the error text (a leak).
func Create(s *store.Store) gin.HandlerFunc { // want Create:"gin-handler-factory"
	return func(c *gin.Context) {
		if err := s.Create(1); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.Status(201)
	}
}

// Lookup is the status-mapper shape: ErrInvalid takes the mapper's status, the
// fallthrough 500 takes the rest (ErrNotFound).
func Lookup(s *store.Store) gin.HandlerFunc { // want Lookup:"gin-handler-factory"
	return func(c *gin.Context) {
		_, err := s.Get(-1)
		if status, ok := statusOf(err); ok {
			c.AbortWithStatusJSON(status, gin.H{"error": "bad"})
			return
		}
		c.JSON(500, gin.H{"error": "unknown"})
	}
}

// Delete is the errors.Is / else shape: ErrNotFound 404, the else 500.
func Delete(s *store.Store) gin.HandlerFunc { // want Delete:"gin-handler-factory"
	return func(c *gin.Context) {
		_, err := s.Get(0)
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(404, gin.H{"error": "gone"})
		} else {
			c.JSON(500, gin.H{"error": "internal"})
		}
	}
}

// Unlock answers an unmapped sentinel with 500 and a 400 with the error text.
func Unlock(s *store.Store) gin.HandlerFunc { // want Unlock:"gin-handler-factory"
	return func(c *gin.Context) {
		if err := s.Lock(); err != nil {
			c.JSON(500, gin.H{"error": "locked"})
			return
		}
		if _, err := s.Get(0); err != nil {
			c.String(400, "bad: %s", err.Error())
		}
	}
}

// Fresh answers a fresh error with 500 (info).
func Fresh(s *store.Store) gin.HandlerFunc { // want Fresh:"gin-handler-factory"
	return func(c *gin.Context) {
		if err := s.Fail(); err != nil {
			c.JSON(500, gin.H{"error": "fail"})
		}
	}
}

// Nested puts the error text two composite literals deep (the kuzma-wms envelope).
func Nested(s *store.Store) gin.HandlerFunc { // want Nested:"gin-handler-factory"
	return func(c *gin.Context) {
		if _, err := s.Get(0); err != nil {
			c.AbortWithStatusJSON(500, gin.H{"detail": gin.H{"error": err.Error()}})
		}
	}
}

// Routes registers the handlers. The group prefix is resolved.
func Routes(r *gin.Engine, s *store.Store) {
	api := r.Group("/api")
	api.GET("/items/1", GetItem(s))
	r.POST("/items", Create(s))
	r.DELETE("/items/:id", Delete(s))
}
