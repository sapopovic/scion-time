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

var (
	refClks        []client.ReferenceClock
	refClkOffsets  []measurements.Measurement
	refClkClient   client.ReferenceClockClient
	peerClks       []client.ReferenceClock
	peerClkOffsets []measurements.Measurement
	peerClkClient  client.ReferenceClockClient
)

func (c *localReferenceClock) MeasureClockOffset(context.Context) (
	time.Time, time.Duration, error) {
	return time.Time{}, 0, nil
}

func (c *localReferenceClock) Drift() (time.Duration, bool) {
	return 0, false
}

func RegisterClocks(refClocks, peerClocks []client.ReferenceClock) {
	if refClks != nil || peerClks != nil {
		panic("reference clocks already registered")
	}

	refClks = refClocks
	refClkOffsets = make([]measurements.Measurement, len(refClks))

	peerClks = peerClocks
	if len(peerClks) != 0 {
		peerClks = append(peerClks, &localReferenceClock{})
	}
	peerClkOffsets = make([]measurements.Measurement, len(peerClks))
}

func measureOffsetToRefClocks(timeout time.Duration) (
	time.Time, time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	refClkClient.MeasureClockOffsets(ctx, refClks, refClkOffsets)
	m := measurements.Median(refClkOffsets)
	return m.Timestamp, m.Offset
}

func SyncToRefClocks(log *slog.Logger, lclk timebase.LocalClock) {
	_, corr := measureOffsetToRefClocks(refClkTimeout)
	if corr != 0 {
		lclk.Step(corr)
	}
}

func RunLocalClockSync(log *slog.Logger, lclk timebase.LocalClock) {
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
	pll := newPLL(log, lclk)
	for {
		corrGauge.Set(0)
		_, corr := measureOffsetToRefClocks(refClkTimeout)
		if corr.Abs() > refClkCutoff {
			if float64(corr.Abs()) > maxCorr {
				corr = time.Duration(float64(timemath.Sgn(corr)) * maxCorr)
			}
			pll.Do(corr, 1000.0 /* weight */)
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(refClkInterval)
	}
}

func measureOffsetToPeerClocks(timeout time.Duration) (
	time.Time, time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	peerClkClient.MeasureClockOffsets(ctx, peerClks, peerClkOffsets)
	m := measurements.FaultTolerantMidpoint(peerClkOffsets)
	return m.Timestamp, m.Offset
}

func driftOfPeerClocks() time.Duration {
	var ds []time.Duration
	for _, peerClk := range peerClks {
		d, ok := peerClk.Drift()
		if ok {
			ds = append(ds, d)
		}
	}
	if len(ds) == 0 {
		return 0.0
	}
	return timemath.FaultTolerantMidpoint(ds)
}

func RunPeerClockSync(log *slog.Logger, lclk timebase.LocalClock) {
	if peerClkImpact <= 1.0 {
		panic("invalid network clock impact factor")
	}
	if peerClkImpact-1.0 <= refClkImpact {
		panic("invalid network clock impact factor")
	}
	if peerClkInterval < refClkInterval {
		panic("invalid network clock sync interval")
	}
	if peerClkTimeout < 0 || peerClkTimeout > peerClkInterval/2 {
		panic("invalid network clock sync timeout")
	}
	maxCorr := peerClkImpact * float64(lclk.MaxDrift(peerClkInterval))
	if maxCorr <= 0 {
		panic("invalid network clock max correction")
	}
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: metrics.SyncNetworkCorrN,
		Help: metrics.SyncNetworkCorrH,
	})
	pll := newPLL(log, lclk)
	for {
		corrGauge.Set(0)
		_, corr := measureOffsetToPeerClocks(peerClkTimeout)
		_ = driftOfPeerClocks()
		if corr.Abs() > peerClkCutoff {
			if float64(corr.Abs()) > maxCorr {
				corr = time.Duration(float64(timemath.Sgn(corr)) * maxCorr)
			}
			pll.Do(corr, 1000.0 /* weight */)
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(peerClkInterval)
	}
}
