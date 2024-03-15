package phc

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type ReferenceClock struct {
	dev string
}

func NewReferenceClock(dev string) *ReferenceClock {
	return &ReferenceClock{dev: dev}
}

func (c *ReferenceClock) MeasureClockOffset(ctx context.Context, log *zap.Logger) (time.Duration, error) {
	panic("not yet implemented")
}
