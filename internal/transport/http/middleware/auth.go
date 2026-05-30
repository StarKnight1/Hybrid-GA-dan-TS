package middleware

import (
	"fmt"
	"net/http"
	"smp_mater_dei_be/internal/platform/config"
	"smp_mater_dei_be/internal/transport/http/response"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")

		if tokenString == "" {
			response.Fail(c, http.StatusUnauthorized, "missing token", nil)
			c.Abort()
			return
		}

		if !strings.HasPrefix(tokenString, "Bearer ") {
			response.Fail(c, http.StatusUnauthorized, "invalid token format", nil)
			c.Abort()
			return
		}

		tokenString = strings.TrimPrefix(tokenString, "Bearer ")

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}

			return []byte(config.JWTSecret()), nil
		})

		if err != nil || !token.Valid {
			response.Fail(c, http.StatusUnauthorized, "invalid token", nil)
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			response.Fail(c, http.StatusUnauthorized, "invalid claims", nil)
			c.Abort()
			return
		}

		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			response.Fail(c, http.StatusUnauthorized, "invalid subject claim", nil)
			c.Abort()
			return
		}

		c.Set("userID", sub)

		if role, ok := claims["role"].(string); ok {
			c.Set("userRole", role)
		}

		c.Next()
	}
}
