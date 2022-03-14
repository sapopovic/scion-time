package core

import (
	"context"
	"fmt"
	"time"
)

type LocalClock interface {
	Now() time.Time
	MaxDrift(duration time.Duration) time.Duration
	Step(offset time.Duration)
	Adjust(offset, duration time.Duration) // TODO: add argument 'frequency float64'
	Sleep(duration time.Duration)
}

type measurement struct {
	off time.Duration
	err error
}

var errNoClockMeasurements = fmt.Errorf("failed to measure clock values")

func collectMeasurements(ctx context.Context, ms chan measurement, n int) []time.Duration {
	i := 0
	var off []time.Duration
loop:
	for i != n {
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
	go func(n int) { // drain channel
		for n != 0 {
			<-ms
			n--
		}
	}(n - i)
	return off
}
