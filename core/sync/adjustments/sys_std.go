//go:build !linux

package adjustments

import (
	"context"
	"log/slog"
	"time"
)

type SysAdjustment struct{}

var _ Adjustment = (*SysAdjustment)(nil)

func (a *SysAdjustment) Do(offset time.Duration) {
	ctx := context.Background()
	log := slog.Default()
	log.LogAttrs(ctx, slog.LevelDebug, "SysAdjustment.Do, not yet implemented",
		slog.Duration("offset", offset))
}
