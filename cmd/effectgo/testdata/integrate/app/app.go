// Package app is the integration fixture: one Gin handler and one Huma route
// that pass the same Huma status-carrying 404 through local error variables.
package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// carry returns the Huma 404 of a missing item: a status-carrying variant.
func carry() error {
	var err error = huma.Error404NotFound("missing")
	return err
}

// ThroughLocal passes the carried 404 through two local variables and writes a
// Gin response with a constant 500. The gin finding must name the Huma variant.
func ThroughLocal(c *gin.Context) {
	err := carry()
	e2 := err
	if e2 != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false})
	}
}

type in struct{}

type out struct{}

// Find returns the carried 404 through local variables. The Huma route answers
// it with its own status (404), so the Huma adapter must report no 5xx finding.
func Find(ctx context.Context, _ *in) (*out, error) {
	err := carry()
	e2 := err
	return nil, e2
}

// Register wires the Huma route and the Gin route.
func Register(api huma.API, r *gin.Engine) {
	huma.Get(api, "/carried", Find)
	r.GET("/through", ThroughLocal)
}
