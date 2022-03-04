package core

import (
	"context"
	"log"
	"time"

	"example.com/scion-time/go/core/timemath"
)

const refClockClientLogPrefix = "[core/clock_ref]"

type TimeSource interface {
	MeasureClockOffset() (time.Duration, error)
}

type ReferenceClockClient struct{}

func (rcc *ReferenceClockClient) MeasureClockOffset(ctx context.Context, tss []TimeSource) (time.Duration, error) {
	type measurement struct {
		off time.Duration
		err error
	}
	ms := make(chan measurement)
	for _, ts := range tss {
		go func(ts TimeSource) {
			off, err := ts.MeasureClockOffset()
			if err != nil {
				log.Printf("%s Failed to fetch clock offset from %v: %v", refClockClientLogPrefix, ts, err)
			}
			ms <- measurement{off, err}
		}(ts)
	}
	i := 0
	var off []time.Duration
loop:
	for i != len(tss) {
		select {
		case m := <-ms:
			if m.err == nil {
				off = append(off, m.off)
			}
			i++
		case <-ctx.Done():
			break loop
		}
	}
	go func(n int) { // drain ms
		for n != 0 {
			<-ms
			n--
		}
	}(len(tss) - i)
	if len(off) == 0 {
		return 0, errNoClockMeasurements
	}
	return timemath.Median(off), nil
}
