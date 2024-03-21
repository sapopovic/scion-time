// Driver for quick experiments

package main

import (
	"time"

	"go.uber.org/zap"

	"example.com/scion-time/base/zaplog"
	"example.com/scion-time/driver/clock"
)

func runX() {
	initLogger(true /* verbose */)

	clk := &clock.SystemClock{Log: zaplog.Logger()}
	zaplog.Logger().Debug("local clock", zap.Stringer("now", clk.Now()))
	clk.Step(-1 * time.Second)
	zaplog.Logger().Debug("local clock", zap.Stringer("now", clk.Now()))
}
