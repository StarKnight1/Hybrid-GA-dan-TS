package logging

import (
	"go.uber.org/zap"
)

var Logger = zap.NewNop()

func InitLogger() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return
	}

	Logger = logger
}
