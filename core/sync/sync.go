package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"golang.org/x/sys/unix"

	"example.com/scion-time/driver/fb/clock"

	"example.com/scion-time/base/metrics"
	"example.com/scion-time/base/timebase"
	"example.com/scion-time/base/timemath"

	"example.com/scion-time/core/client"
	"example.com/scion-time/core/measurements"
	"example.com/scion-time/core/servo"
)

const (
	refClkImpact   = 1.25
	refClkCutoff   = 0
	refClkTimeout  = 1 * time.Second
	refClkInterval = 2 * time.Second
	netClkImpact   = 2.5
	netClkCutoff   = time.Microsecond
	netClkTimeout  = 5 * time.Second
	netClkInterval = 60 * time.Second
)

type localReferenceClock struct{}

var (
	refClks       []client.ReferenceClock
	refClkOffsets []measurements.Measurement
	refClkClient  client.ReferenceClockClient
	netClks       []client.ReferenceClock
	netClkOffsets []measurements.Measurement
	netClkClient  client.ReferenceClockClient
)

func (c *localReferenceClock) MeasureClockOffset(context.Context) (
	time.Time, time.Duration, error) {
	return time.Time{}, 0, nil
}

func RegisterClocks(refClocks, netClocks []client.ReferenceClock) {
	if refClks != nil || netClks != nil {
		panic("reference clocks already registered")
	}

	refClks = refClocks
	refClkOffsets = make([]measurements.Measurement, len(refClks))

	netClks = netClocks
	if len(netClks) != 0 {
		netClks = append(netClks, &localReferenceClock{})
	}
	netClkOffsets = make([]measurements.Measurement, len(netClks))
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

func initServo() (*servo.PiServo, error) {
	/*
		Copyright (c) Facebook, Inc. and its affiliates.

		Licensed under the Apache License, Version 2.0 (the "License");
		you may not use this file except in compliance with the License.
		You may obtain a copy of the License at

		    http://www.apache.org/licenses/LICENSE-2.0

		Unless required by applicable law or agreed to in writing, software
		distributed under the License is distributed on an "AS IS" BASIS,
		WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
		See the License for the specific language governing permissions and
		limitations under the License.
	*/

	freqPPB, clkstate, err := clock.FrequencyPPB(unix.CLOCK_REALTIME)
	if err != nil {
		return nil, err
	}
	if clkstate != unix.TIME_OK {
		slog.Info("clock state is not TIME_OK after getting current frequency", "clkstate", clkstate)
	}
	slog.Debug("starting CLOCK_REALTIME frequency", "freqPPB", freqPPB)
	maxFreqPPB, clkstate, err := clock.MaxFreqPPB(unix.CLOCK_REALTIME)
	if err != nil {
		return nil, err
	}
	if clkstate != unix.TIME_OK {
		slog.Info("clock state is not TIME_OK after getting current frequency", "clkstate", clkstate)
	}
	slog.Debug("max CLOCK_REALTIME frequency", "maxFreqPPB", maxFreqPPB)

	servoCfg := servo.DefaultServoConfig()
	servoCfg.FirstUpdate = true
	servoCfg.FirstStepThreshold = int64(1 * time.Second)
	pi := servo.NewPiServo(servoCfg, servo.DefaultPiServoCfg(), -freqPPB)
	pi.SyncInterval(2 * time.Second.Seconds())
	pi.SetMaxFreq(maxFreqPPB)
	piFilterCfg := servo.DefaultPiServoFilterCfg()
	_ = servo.NewPiServoFilter(pi, piFilterCfg)
	return pi, nil
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
	pi, err := initServo()
	if err != nil {
		panic(err)
	}
	// pll := newPLL(log, lclk)
	for {
		corrGauge.Set(0)
		ts, corr := measureOffsetToRefClocks(refClkTimeout)
		if timemath.Abs(corr) > refClkCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			// lclk.Adjust(corr, refClkInterval, 0)
			// pll.Do(corr, 1000.0 /* weight */)
			log.Debug("corr", "val", -int64(corr))
			freqAdj, state := pi.Sample(-int64(corr), uint64(ts.UnixNano()))
			if state == servo.StateJump {
				log.Debug("Step", "corr", -corr)
				_, err := clock.Step(unix.CLOCK_REALTIME, -corr)
				if err != nil {
					log.Error("failed to step clock", "step", -corr, "error", err)
					continue
				}
			} else {
				log.Debug("AdjFreqPPB", "freqAdj", -freqAdj)
				_, err := clock.AdjFreqPPB(unix.CLOCK_REALTIME, -freqAdj)
				if err != nil {
					log.Error("failed to adjust clock freq", "freqAdj", -freqAdj, "error", err)
					continue
				}
				if err := clock.SetSync(unix.CLOCK_REALTIME); err != nil {
					log.Error("failed to set sys clock sync state", "error", err)
				}
			}
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(refClkInterval)
	}
}

func measureOffsetToNetClocks(timeout time.Duration) (
	time.Time, time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	netClkClient.MeasureClockOffsets(ctx, netClks, netClkOffsets)
	m := measurements.FaultTolerantMidpoint(netClkOffsets)
	return m.Timestamp, m.Offset
}

func RunGlobalClockSync(log *slog.Logger, lclk timebase.LocalClock) {
	if netClkImpact <= 1.0 {
		panic("invalid network clock impact factor")
	}
	if netClkImpact-1.0 <= refClkImpact {
		panic("invalid network clock impact factor")
	}
	if netClkInterval < refClkInterval {
		panic("invalid network clock sync interval")
	}
	if netClkTimeout < 0 || netClkTimeout > netClkInterval/2 {
		panic("invalid network clock sync timeout")
	}
	maxCorr := netClkImpact * float64(lclk.MaxDrift(netClkInterval))
	if maxCorr <= 0 {
		panic("invalid network clock max correction")
	}
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: metrics.SyncGlobalCorrN,
		Help: metrics.SyncGlobalCorrH,
	})
	pll := newPLL(log, lclk)
	for {
		corrGauge.Set(0)
		_, corr := measureOffsetToNetClocks(netClkTimeout)
		if timemath.Abs(corr) > netClkCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			// lclk.Adjust(corr, netClkInterval, 0)
			pll.Do(corr, 1000.0 /* weight */)
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(netClkInterval)
	}
}
