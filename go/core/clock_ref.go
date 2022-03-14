package core

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"example.com/scion-time/go/core/timemath"
)

const refClockClientLogPrefix = "[core/clock_ref]"

type TimeSource interface {
	MeasureClockOffset(ctx context.Context) (time.Duration, error)
}

type ReferenceClockClient struct {
	numOpsInProgress uint32
}

func (rcc *ReferenceClockClient) MeasureClockOffset(ctx context.Context,
	tss []TimeSource) (time.Duration, error) {
	swapped := atomic.CompareAndSwapUint32(&rcc.numOpsInProgress, 0, 1)
	if !swapped {
		panic("too many reference clock offset measurements in progress")
	}
	defer func(addr *uint32) {
		swapped := atomic.CompareAndSwapUint32(addr, 1, 0)
		if !swapped {
			panic("inconsistent count of reference clock offset measurements")
		}
	}(&rcc.numOpsInProgress)

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
