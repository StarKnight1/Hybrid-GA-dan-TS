package subjects

import (
	"net/http"
	"strconv"

	"smp_mater_dei_be/internal/transport/http/response"

	"github.com/gin-gonic/gin"
)

type subjectRequest struct {
	Name string `json:"name" binding:"required"`
}

func GetSubjectsHandler(c *gin.Context) {
	list, err := GetAllSubjects()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to get subjects", err.Error())
		return
	}
	response.OK(c, list, "subjects retrieved successfully")
}

func CreateSubjectHandler(c *gin.Context) {
	var req subjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request", err.Error())
		return
	}

	s, err := CreateSubject(req.Name)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to create subject", err.Error())
		return
	}
	response.Created(c, s, "subject created successfully")
}

func UpdateSubjectHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id", err.Error())
		return
	}

	var req subjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request", err.Error())
		return
	}

	s, err := UpdateSubject(uint(id), req.Name)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to update subject", err.Error())
		return
	}
	response.OK(c, s, "subject updated successfully")
}

func DeleteSubjectHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id", err.Error())
		return
	}

	if err := DeleteSubject(uint(id)); err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to delete subject", err.Error())
		return
	}
	response.OK(c, nil, "subject deleted successfully")
}
