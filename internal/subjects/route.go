package subjects

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.Engine) {
	s := r.Group("/subjects")
	s.GET("", GetSubjectsHandler)
	s.POST("", CreateSubjectHandler)
	s.PUT("/:id", UpdateSubjectHandler)
	s.DELETE("/:id", DeleteSubjectHandler)
}
