package gatepaths

import "github.com/gin-gonic/gin"

func Handle(*gin.Context) {}

func Empty(r *gin.Engine) {
	r.GET("", Handle)
}

func Dynamic(r *gin.Engine, prefix string) {
	group := r.Group(prefix)
	group.GET("/items", Handle)
}

func Nested(r *gin.Engine, prefix string) {
	r.Group(prefix).Group("/v1").GET("/items", Handle)
}
