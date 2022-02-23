package core

import (
	"context"
	"time"
)

const netClockLogPrefix = "[core/clock_net]"

type NetworkClockClient struct {}

func (ncc *NetworkClockClient) MeasureClockOffset(ctx context.Context, pi PathInfo) (time.Duration, error) {
	return 0, nil
}
