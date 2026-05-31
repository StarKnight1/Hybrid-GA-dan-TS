package admin

import (
	"smp_mater_dei_be/internal/transport/http/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	g := r.Group("/admin")
	g.Use(middleware.AuthMiddleware())
	g.Use(middleware.RequireRole("admin"))

	g.GET("/template", DownloadTemplateHandler)
	g.POST("/upload", UploadDataHandler)
	g.DELETE("/data", ClearDataHandler)
	g.GET("/data-status", DataStatusHandler)
}
