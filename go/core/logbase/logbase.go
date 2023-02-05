package logbase

import (
	"go.uber.org/zap"
)

func init() {
	logger := zap.Must(zap.NewDevelopment())
	_ = zap.ReplaceGlobals(logger)
}

func L() *zap.Logger {
	return zap.L()
}
