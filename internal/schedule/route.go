package schedule

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.Engine) {
	scheduleRoutes := r.Group("/schedule")
	scheduleRoutes.GET("/ga-info", GAInfoHandler)
	scheduleRoutes.GET("/generate/stream", GenerateV3ScheduleStreamHandler)
	scheduleRoutes.GET("/generate/multi-run", GenerateV3MultiRunHandler)
	scheduleRoutes.GET("/generate/multi-run/stream", GenerateV3MultiRunStreamHandler)
}
