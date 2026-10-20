package empty

import "github.com/gin-gonic/gin"

func Handle(*gin.Context) {}

func Register(r *gin.Engine) {
	r.GET("", Handle)
}
