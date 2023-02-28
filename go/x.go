// Driver for quick experiments

package main

import (
	"context"
	"time"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/driver/clock"
	"example.com/scion-time/go/driver/mbg"
)

func runX() {
	initLogger(true /* verbose */)
	ctx := context.Background()

	lclk := &clock.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	for {
		_, _ = mbg.MeasureClockOffset(ctx, log, "/dev/mbgclock0")
		lclk.Sleep(1 * time.Second)
	}
}
