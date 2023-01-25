// Driver for quick experiments

package main

import (
	"context"
	"time"

	"example.com/scion-time/go/core"
	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/driver/mbg"
)

func runX() {
	ctx := context.Background()
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)
	for {
		mbg.MeasureClockOffset(ctx, "/dev/mbgclock0")
		lclk.Sleep(1 * time.Second)
	}
}
