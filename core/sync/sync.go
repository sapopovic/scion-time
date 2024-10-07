package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"example.com/scion-time/base/metrics"
	"example.com/scion-time/base/timebase"
	"example.com/scion-time/base/timemath"

	"example.com/scion-time/core/client"
	"example.com/scion-time/core/measurements"
	"example.com/scion-time/core/sync/adjustments"
)

const (
	refClkImpact  = 1.25

	peerClkImpact = 2.5
	peerClkCutoff = 50 * time.Microsecond

	syncTimeout   = 500 * time.Millisecond
	syncInterval  = 1000 * time.Millisecond
)

type localReferenceClock struct{}

func (c *localReferenceClock) MeasureClockOffset(context.Context) (
	time.Time, time.Duration, error) {
	return time.Time{}, 0, nil
}

func measureOffsetToRefClks(refClkClient client.ReferenceClockClient,
	refClks []client.ReferenceClock, refClkOffsets []measurements.Measurement,
	timeout time.Duration) (time.Time, time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	refClkClient.MeasureClockOffsets(ctx, refClks, refClkOffsets)
	m := measurements.FaultTolerantMidpoint(refClkOffsets)
	return m.Timestamp, m.Offset
}

func Run(log *slog.Logger,
	clk timebase.SystemClock, adj adjustments.Adjustment,
	refClks, peerClks []client.ReferenceClock) {
	ctx := context.Background()
	if refClkImpact <= 1.0 {
		panic("invalid local reference clock impact factor")
	}
	if peerClkImpact <= 1.0 {
		panic("invalid peer clock impact factor")
	}
	if peerClkImpact-1.0 <= refClkImpact {
		panic("invalid peer clock impact factor")
	}
	if syncInterval <= 0 {
		panic("invalid sync interval")
	}
	if syncTimeout < 0 || syncTimeout > syncInterval/2 {
		panic("invalid sync timeout")
	}
	refClkMaxCorr := refClkImpact * float64(clk.Drift(syncInterval))
	if refClkMaxCorr <= 0 {
		panic("unexpected system clock behavior")
	}
	peerClkMaxCorr := peerClkImpact * float64(clk.Drift(syncInterval))
	if peerClkMaxCorr <= 0 {
		panic("unexpected system clock behavior")
	}
	var refClkClient client.ReferenceClockClient
	refClkOffsets := make([]measurements.Measurement, len(refClks))
	refClkOffCh := make(chan time.Duration)
	if len(peerClks) != 0 {
		peerClks = append(peerClks, &localReferenceClock{})
	}
	var peerClkClient client.ReferenceClockClient
	peerClkOffsets := make([]measurements.Measurement, len(peerClks))
	peerClkOffCh := make(chan time.Duration)
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: metrics.SyncCorrN,
		Help: metrics.SyncCorrH,
	})
	corrGauge.Set(0)
	for {
		go func() {
			var refClkOff time.Duration
			if len(refClks) != 0 {
				_, refClkOff = measureOffsetToRefClks(
					refClkClient, refClks, refClkOffsets, syncTimeout)
			}
			refClkOffCh <- refClkOff
		}()
		go func() {
			var peerClkOff time.Duration
			if len(peerClks) != 0 {
				_, peerClkOff = measureOffsetToRefClks(
					peerClkClient, peerClks, peerClkOffsets, syncTimeout)
			}
			peerClkOffCh <- peerClkOff
		}()
		refClkOff, peerClkOff := <-refClkOffCh, <-peerClkOffCh
		refClkCorr, peerClkCorr := refClkOff, peerClkOff
		var refClkOk bool
		if float64(refClkCorr.Abs()) > refClkMaxCorr {
			refClkCorr = time.Duration(float64(timemath.Sgn(refClkCorr)) * refClkMaxCorr)
		}
		refClkOk = len(refClks) != 0
		var peerClkOk bool
		if peerClkCorr.Abs() > peerClkCutoff {
			if float64(peerClkCorr.Abs()) > peerClkMaxCorr {
				peerClkCorr = time.Duration(float64(timemath.Sgn(peerClkCorr)) * peerClkMaxCorr)
			}
			peerClkOk = len(peerClks) != 0
		}
		var corr time.Duration
		switch {
		case refClkOk && !peerClkOk:
			corr = refClkCorr
		case !refClkOk && peerClkOk:
			corr = peerClkCorr
		case refClkOk && peerClkOk:
			corr = timemath.Midpoint(refClkCorr, peerClkCorr)
		}
		log.LogAttrs(ctx, slog.LevelDebug, "correcting clock",
			slog.Float64("corr", corr.Seconds()),
			slog.Bool("refClkOk", refClkOk),
			slog.Float64("refClkOff", refClkOff.Seconds()),
			slog.Float64("refClkCorr", refClkCorr.Seconds()),
			slog.Float64("refClkMaxCorr", float64(refClkMaxCorr) / 1e9),
			slog.Bool("peerClkOk", peerClkOk),
			slog.Float64("peerClkOff", peerClkOff.Seconds()),
			slog.Float64("peerClkCorr", peerClkCorr.Seconds()),
			slog.Float64("peerClkMaxCorr", float64(peerClkMaxCorr) / 1e9),
			slog.Float64("peerClkCutoff", peerClkCutoff.Seconds()))
		adj.Do(corr)
		corrGauge.Set(float64(corr))
		clk.Sleep(syncInterval)
	}
}
