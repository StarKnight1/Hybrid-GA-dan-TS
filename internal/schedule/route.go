package schedule

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.Engine) {
	scheduleRoutes := r.Group("/schedule")
	scheduleRoutes.GET("/ga-info", GAInfoHandler)
	scheduleRoutes.GET("/diagnose/sa", DiagnoseSAHandler)
	scheduleRoutes.GET("/diagnose/matrix-slots", DiagnoseMatrixSlotsHandler)
	scheduleRoutes.GET("/diagnose/matrix-blocks", DiagnoseMatrixBlocksHandler)
	scheduleRoutes.GET("/real/validate", ValidateRealScheduleHandler)
	scheduleRoutes.GET("/generate", GenerateScheduleHandler)
	scheduleRoutes.GET("/generate/compare-real", GenerateScheduleAndCompareHandler)
	scheduleRoutes.GET("/generate/stream", GenerateScheduleStreamHandler)
	scheduleRoutes.GET("/generate/compare-real/stream", GenerateScheduleAndCompareStreamHandler)
	scheduleRoutes.GET("/generate/v2/stream", GenerateV2ScheduleStreamHandler)
	scheduleRoutes.GET("/generate/v3/stream", GenerateV3ScheduleStreamHandler)
	scheduleRoutes.GET("/generate/v3/readable", GenerateV3ScheduleReadableHandler)
	scheduleRoutes.GET("/generate/v3/multi-run", GenerateV3MultiRunHandler)
	scheduleRoutes.GET("/generate/v3/multi-run/stream", GenerateV3MultiRunStreamHandler)
	scheduleRoutes.GET("/generate/v3/compare-real/stream", GenerateV3ScheduleStreamHandler)
}
