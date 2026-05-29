package users

import (
	"smp_mater_dei_be/internal/transport/http/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	authRoutes := r.Group("/auth")
	authRoutes.POST("/login", LoginHandler)

	userRoutes := r.Group("/users")
	userRoutes.Use(middleware.AuthMiddleware())
	userRoutes.GET("/me", GetProfileHandler)
}
