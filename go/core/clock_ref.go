package core

import (
	"context"
	"crypto/rand"
	"log"
	"sort"
	"time"
)

const refClockLogPrefix = "[core/clock_ref]"

type TimeSource interface {
	FetchTime() (refTime time.Time, sysTime time.Time, err error)
}

type ReferenceClockClient struct {}

func medianTimeInfo(tis []time.Duration) time.Duration {
	sort.Slice(tis, func(i, j int) bool {
		return tis[i] < tis[j]
	})
	var m time.Duration
	n := len(tis)
	if n == 0 {
		m = 0
	} else {
		i := n / 2
		if n%2 != 0 {
			m = tis[i]
		} else {
			b := make([]byte, 1)
			_, err  := rand.Read(b)
			if err != nil {
				log.Fatalf("%s Failed to read random number: %v", refClockLogPrefix, err)
			}
			if b[0] > 127 {
				m = tis[i]
			} else {
				m = tis[i-1]
			}
		}
	}
	return m
}

func (r *ReferenceClockClient) MeasureClockOffset(ctx context.Context, tss []TimeSource) (time.Duration, error) {
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
