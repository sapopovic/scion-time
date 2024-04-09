package server

import (
	"context"
	"log/slog"
	"os"
)

func logFatal(ctx context.Context, log *slog.Logger, msg string, attrs ...slog.Attr) {
	log.LogAttrs(ctx, slog.LevelError, msg, attrs...)
	os.Exit(1)
}
