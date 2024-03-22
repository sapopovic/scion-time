// Driver for quick experiments

package main

import (
	"log/slog"
	"time"

	"example.com/scion-time/driver/clock"
)

func runX() {
	initLogger(true /* verbose */)

	log := slog.Default()

	clk := &clock.SystemClock{Log: log}
	log.Debug("local clock", slog.Time("now", clk.Now()))
	clk.Step(-1 * time.Second)
	log.Debug("local clock", slog.Time("now", clk.Now()))
}
