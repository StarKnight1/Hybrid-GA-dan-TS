package middleware

import (
	"net/http"
	"smp_mater_dei_be/internal/transport/http/response"

	"github.com/gin-gonic/gin"
)

// RequireRole blocks requests whose JWT role claim is not in the allowed list.
// Must be used after AuthMiddleware().
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleAny, exists := c.Get("userRole")
		if !exists {
			response.Fail(c, http.StatusForbidden, "role not found", nil)
			c.Abort()
			return
		}
		userRole, _ := roleAny.(string)
		for _, r := range roles {
			if userRole == r {
				c.Next()
				return
			}
		}
		response.Fail(c, http.StatusForbidden, "insufficient permissions", nil)
		c.Abort()
	}
}
