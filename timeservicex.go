// Driver for quick experiments

package main

import (
	"context"
	"log/slog"
	"time"

	"example.com/scion-time/driver/clocks"
	_ "example.com/scion-time/net/csptp"
	"example.com/scion-time/net/ntp"
)

func TimeFromTime64(t ntp.Time64, t0 time.Time) time.Time {
	const (
		epoch         = -(70*365 + 17) * 86400 // Seconds from Unix epoch (1970) to NTP epoch (1900), incl. leap days
		secondsPerEra = 1 << 32
	)

	tref := t0.Unix()

	sec := epoch + (tref-epoch)/secondsPerEra*secondsPerEra + int64(t.Seconds)

	// If the timestamp would be too far in the past relative to
	// the reference time, assume it's from the next era
	if sec < tref-secondsPerEra/2 {
		sec += secondsPerEra
	}

	return time.Unix(sec, 0)
}

func runX() {
	initLogger(true /* verbose */)

	log := slog.Default()

	clk := clocks.NewSystemClock(log, clocks.UnknownDrift)
	log.Debug("local clock", slog.Time("now", clk.Now()))
	clk.Step(-1 * time.Second)
	log.Debug("local clock", slog.Time("now", clk.Now()))

	now := time.Now().UTC()

	now64 := ntp.Time64FromTime(now)
	log.LogAttrs(context.Background(), slog.LevelDebug, "test",
		slog.Any("now", ntp.Time64LogValuer{T: now64}))

	t0 := TimeFromTime64(ntp.Time64{Seconds: 1<<32 - 1, Fraction: 0}, now)
	t1 := TimeFromTime64(ntp.Time64{Seconds: 0, Fraction: 0}, now)
	log.LogAttrs(context.Background(), slog.LevelDebug, "test",
		slog.Time("t0", t0),
		slog.Time("t1", t1),
	)

	var pkt ntp.Packet
	log.LogAttrs(context.Background(), slog.LevelDebug, "test",
		slog.Any("pkt", ntp.PacketLogValuer{Pkt: &pkt}))
}
