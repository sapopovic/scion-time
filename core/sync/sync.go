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
	refClkImpact    = 1.25
	refClkCutoff    = 0
	refClkTimeout   = 1 * time.Second
	refClkInterval  = 2 * time.Second
	peerClkImpact   = 2.5
	peerClkCutoff   = time.Microsecond
	peerClkTimeout  = 5 * time.Second
	peerClkInterval = 60 * time.Second
)

type localReferenceClock struct{}

func (c *localReferenceClock) MeasureClockOffset(context.Context) (
	time.Time, time.Duration, error) {
	return time.Time{}, 0, nil
}

type pllAdjustment struct {
	pll *adjustments.Pll
}

func (a *pllAdjustment) Do(offset time.Duration) {
	a.pll.Do(offset, 1000.0 /* weight */)
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

func RunLocalClockSync(log *slog.Logger,
	lclk timebase.LocalClock, adj adjustments.Adjustment,
	refClks []client.ReferenceClock) {
	if refClkImpact <= 1.0 {
		panic("invalid reference clock impact factor")
	}
	if refClkInterval <= 0 {
		panic("invalid reference clock sync interval")
	}
	if refClkTimeout < 0 || refClkTimeout > refClkInterval/2 {
		panic("invalid reference clock sync timeout")
	}
	maxCorr := refClkImpact * float64(lclk.MaxDrift(refClkInterval))
	if maxCorr <= 0 {
		panic("invalid reference clock max correction")
	}
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: metrics.SyncLocalCorrN,
		Help: metrics.SyncLocalCorrH,
	})
	var refClkClient client.ReferenceClockClient
	refClkOffsets := make([]measurements.Measurement, len(refClks))
	if adj == nil {
		adj = &pllAdjustment{pll: adjustments.NewPLL(log, lclk)}
	}
	for {
		corrGauge.Set(0)
		_, corr := measureOffsetToRefClks(refClkClient, refClks, refClkOffsets, refClkTimeout)
		if corr.Abs() > refClkCutoff {
			if float64(corr.Abs()) > maxCorr {
				corr = time.Duration(float64(timemath.Sgn(corr)) * maxCorr)
			}
			adj.Do(corr)
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(refClkInterval)
	}
}

func RunPeerClockSync(log *slog.Logger,
	lclk timebase.LocalClock, adj adjustments.Adjustment,
	peerClks []client.ReferenceClock) {
	if peerClkImpact <= 1.0 {
		panic("invalid peer clock impact factor")
	}
	if peerClkImpact-1.0 <= refClkImpact {
		panic("invalid peer clock impact factor")
	}
	if peerClkInterval < refClkInterval {
		panic("invalid peer clock sync interval")
	}
	if peerClkTimeout < 0 || peerClkTimeout > peerClkInterval/2 {
		panic("invalid peer clock sync timeout")
	}
	maxCorr := peerClkImpact * float64(lclk.MaxDrift(peerClkInterval))
	if maxCorr <= 0 {
		panic("invalid peer clock max correction")
	}
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: metrics.SyncNetworkCorrN,
		Help: metrics.SyncNetworkCorrH,
	})
	var peerClkClient client.ReferenceClockClient
	if len(peerClks) != 0 {
		peerClks = append(peerClks, &localReferenceClock{})
	}
	peerClkOffsets := make([]measurements.Measurement, len(peerClks))
	if adj == nil {
		adj = &pllAdjustment{pll: adjustments.NewPLL(log, lclk)}
	}
	for {
		corrGauge.Set(0)
		_, corr := measureOffsetToRefClks(
			peerClkClient, peerClks, peerClkOffsets, peerClkTimeout)
		if corr.Abs() > peerClkCutoff {
			if float64(corr.Abs()) > maxCorr {
				corr = time.Duration(float64(timemath.Sgn(corr)) * maxCorr)
			}
			adj.Do(corr)
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(peerClkInterval)
	}
}
