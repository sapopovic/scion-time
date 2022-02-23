package core

import (
	"context"
	"log"
	"time"
)

const refClockLogPrefix = "[core/clock_ref]"

type TimeSource interface {
	MeasureClockOffset() (time.Duration, error)
}

type ReferenceClockClient struct {}

func (rcc *ReferenceClockClient) MeasureClockOffset(ctx context.Context, tss []TimeSource) (time.Duration, error) {
	type measurement struct {
		off time.Duration
		err error
	} 
	var ms chan measurement
	for _, ts := range tss {
		go func(ts TimeSource) {
			off, err := ts.MeasureClockOffset()
			if err != nil {
				log.Printf("Failed to fetch clock offset from %v: %v", ts, err)
			}
			ms <- measurement{off, err}
		}(ts)
	}
	var off []time.Duration
	loop:
		for i := 0; i != len(tss); i++ {
			select {
			case m := <-ms:
				if m.err != nil {
					off = append(off, m.off)
				}
			case <-ctx.Done():
				break loop
			}
		}
	if len(off) == 0 {
		return 0, nil
	}
	return Median(off), nil
}
