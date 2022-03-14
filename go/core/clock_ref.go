package core

import (
	"context"
	"log"
	"time"

	"example.com/scion-time/go/core/timemath"
)

const refClockClientLogPrefix = "[core/clock_ref]"

type TimeSource interface {
	MeasureClockOffset(ctx context.Context) (time.Duration, error)
}

type ReferenceClockClient struct{}

func (rcc *ReferenceClockClient) MeasureClockOffset(ctx context.Context,
	tss []TimeSource) (time.Duration, error) {
	ms := make(chan measurement)
	for _, ts := range tss {
		go func(ctx context.Context, ts TimeSource) {
			off, err := ts.MeasureClockOffset(ctx)
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v: %v",
					refClockClientLogPrefix, ts, err)
			}
			ms <- measurement{off, err}
		}(ctx, ts)
	}
	off := collectMeasurements(ctx, ms, len(tss))
	if len(off) == 0 {
		return 0, errNoClockMeasurements
	}
	return timemath.Median(off), nil
}
