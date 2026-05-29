package users

import (
	"net/http"
	"smp_mater_dei_be/internal/transport/http/response"
	"smp_mater_dei_be/internal/users/dto"

	"github.com/gin-gonic/gin"
)

func LoginHandler(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request", err.Error())
		return
	}

	token, err := Login(req.Identifier, req.Password)
	if err != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid credentials", err.Error())
		return
	}

	response.OK(c, gin.H{"token": token}, "login successful")
}

func GetProfileHandler(c *gin.Context) {
	userIDAny, exists := c.Get("userID")
	if !exists {
		response.Fail(c, http.StatusUnauthorized, "user not authenticated", nil)
		return
	}

	userID, ok := userIDAny.(string)
	if !ok {
		response.Fail(c, http.StatusUnauthorized, "invalid user ID", nil)
		return
	}

	profile, err := GetProfile(userID)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to get profile", err.Error())
		return
	}

	response.OK(c, profile, "profile retrieved successfully")
}
