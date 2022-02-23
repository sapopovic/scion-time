package core

import (
	"context"
	"time"
)

const refClockLogPrefix = "[core/clock_ref]"

type TimeSource interface {
	FetchTime() (refTime time.Time, sysTime time.Time, err error)
}

type ReferenceClockClient struct {}

func (rcc *ReferenceClockClient) MeasureClockOffset(ctx context.Context, tss []TimeSource) (time.Duration, error) {
	// for _, ts := range tss {
	// 	go func(ts TimeSource) {
	// 		refTime, sysTime, err := ts.FetchTime()
	// 		if err != nil {
	// 			log.Printf("Failed to fetch clock offset from %v: %v", ts, err)
	// 			off <- 0
	// 			return
	// 		}
	// 		sis <- refTime.Sub(sysTime)
	// 	}(s)
	// }
	// for i := 0; i != len(timeSources); i++ {
	// 	x := <-ch
	// 	if !x.refTime.IsZero() || !x.sysTime.IsZero() {
	// 		tis = append(tis, x)
	// 	}
	// }
	// m := medianTimeInfo(tis)
	// log.Printf("Fetched local time info: refTime: %v, sysTime: %v", m.refTime, m.sysTime)
	return 0, nil
}
