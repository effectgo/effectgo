// Package httpcontract isolates HTTP policy findings from native contracts.
package httpcontract

import (
	"errors"

	"github.com/gin-gonic/gin"
)

var ErrDomain = errors.New("domain failure")

func DomainError() error { return ErrDomain }
func FreshError() error  { return errors.New("fresh") }

func Domain(c *gin.Context) {
	if err := DomainError(); err != nil {
		c.JSON(500, gin.H{"failed": true})
	}
}

func Fresh(c *gin.Context) {
	if err := FreshError(); err != nil {
		c.JSON(500, gin.H{"failed": true})
	}
}

func Register(r *gin.Engine, path string) {
	r.GET("/domain", Domain)
	r.GET("/fresh", Fresh)
	r.GET(path, Domain)
}
