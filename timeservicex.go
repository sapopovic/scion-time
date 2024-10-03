// Driver for quick experiments

package main

import (
	"context"
	"log/slog"
	"time"

	"example.com/scion-time/driver/clocks"
	"example.com/scion-time/net/ntp"
)

func runX() {
	initLogger(true /* verbose */)

	log := slog.Default()

	clk := clocks.NewSystemClock(log, clocks.UnknownDrift)
	log.Debug("local clock", slog.Time("now", clk.Now()))
	clk.Step(-1 * time.Second)
	log.Debug("local clock", slog.Time("now", clk.Now()))

	now64 := ntp.Time64FromTime(time.Now())
	log.LogAttrs(context.Background(), slog.LevelDebug, "test",
		slog.Any("now", ntp.Time64LogValuer{T: now64}))

	var pkt ntp.Packet
	log.LogAttrs(context.Background(), slog.LevelDebug, "test",
		slog.Any("pkt", ntp.PacketLogValuer{Pkt: &pkt}))
}
