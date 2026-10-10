// Package httpapi is a project response helper: the sink rule of the project
// stubs names Detail (status argument 1, body argument 2).
package httpapi

import "github.com/gin-gonic/gin"

func Detail(c *gin.Context, status int, detail string) {
	c.AbortWithStatusJSON(status, gin.H{"detail": detail})
}
