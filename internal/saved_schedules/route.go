package savedschedules

import (
	"smp_mater_dei_be/internal/transport/http/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	g := r.Group("/schedules")
	g.Use(middleware.AuthMiddleware())

	g.GET("", ListHandler)
	g.GET("/active", GetActiveHandler) // harus didaftarkan sebelum /:id
	g.GET("/:id", GetHandler)
	g.GET("/:id/export", ExportHandler)

	// admin-only
	admin := g.Group("")
	admin.Use(middleware.RequireRole("admin"))
	admin.POST("", SaveHandler)
	admin.DELETE("/:id", DeleteHandler)
	admin.PUT("/:id/deploy", DeployHandler)
}
