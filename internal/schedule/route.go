package schedule

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.Engine) {
	s := r.Group("/schedule")
	s.GET("/diagnose/matrix-slots", DiagnoseMatrixSlotsHandler)
	s.GET("/diagnose/matrix-blocks", DiagnoseMatrixBlocksHandler)
	s.GET("/generate/v2/stream", GenerateV2ScheduleStreamHandler)
	s.GET("/generate/v3/stream", GenerateV3ScheduleStreamHandler)
	s.GET("/generate/v3/readable", GenerateV3ScheduleReadableHandler)
	s.GET("/generate/v3/multi-run", GenerateV3MultiRunHandler)
	s.GET("/generate/v3/multi-run/stream", GenerateV3MultiRunStreamHandler)
}
