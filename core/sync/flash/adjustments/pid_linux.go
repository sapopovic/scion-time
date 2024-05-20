/*
 * Based on flashptpd, https://github.com/meinberg-sync/flashptpd
 *
 * @file pidController.h and pidController.cpp
 * @note Copyright 2023, Meinberg Funkuhren GmbH & Co. KG, All rights reserved.
 * @author Thomas Behn <thomas.behn@meinberg.de>
 *
 * PID controller adjustment algorithm. Unlike many other PID controller
 * implementations, this one applies an integral adjustment part by keeping a
 * small part of the previous adjustment when performing a new adjustment with a
 * proportional and (optional) differential part. Ratios of all parts (iRatio,
 * pRatio, dRatio) as well as a step threshold in nanoseconds can be configured,
 * individually.
 *
 * Minimum:     p = 0.01, i = 0.005, d = 0.0
 * Maximum:     p = 1.0, i = 0.5, d = 1.0
 * Default:     p = 0.2, i = 0.05, d = 0.0
 *
 * =============================================================================
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the “Software”),
 * to deal in the Software without restriction, including without limitation
 * the rights to use, copy, modify, merge, publish, distribute, sublicense,
 * and/or sell copies of the Software, and to permit persons to whom the Software
 * is furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND, EXPRESS
 * OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
 * THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
 * IN THE SOFTWARE.
 *
 * =============================================================================
 *
 */

package adjustments

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/sys/unix"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/base/unixutil"
)

const (
	PIDControllerMinPRatio     = 0.01
	PIDControllerDefaultPRatio = 0.2
	PIDControllerMaxPRatio     = 1.0
	PIDControllerMinIRatio     = 0.005
	PIDControllerDefaultIRatio = 0.05
	PIDControllerMaxIRatio     = 0.5
	PIDControllerMinDRatio     = 0.0
	PIDControllerDefaultDRatio = 0.0
	PIDControllerMaxDRatio     = 1.0

	PIDControllerStepThresholdDefault = 1 * time.Millisecond
)

type PIDController struct {
	// Ratio (gain factor) of the proportional control output value (applied to
	// the measured offset).
	KP float64

	// Ratio of the integral control output value. In this PID controller
	// implementation, the integral value is applied by reverting only a part of
	// the previous adjustment. This ratio defines the part of the previous
	// adjustment that is to be kept. That means, that the size of the integral
	// control output depends on all of the configurable ratios (kp, ki or kd) of
	// the PID controller.
	KI float64

	// Ratio of the differential control output value (applied to the measured
	// drift).
	KD float64

	// Offset threshold (ns) indicating that - if exceeded - a clock step is to be
	// applied
	StepThreshold time.Duration

	p, i, d float64

	freqAddend float64
}

var _ Adjustment = (*PIDController)(nil)

func freqOffset(offset time.Duration) float64 {
	return offset.Seconds()
}

func (c *PIDController) Do(offset, drift time.Duration) {
	ctx := context.Background()
	log := slog.Default()

	tx := unix.Timex{}
	_, err := unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
	if err != nil {
		logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
	}
	freq := unixutil.FreqFromScaledPPM(tx.Freq)

	// "fake integral" (partial reversion of previous adjustment)
	// Summing up 'integral' for logging purpose
	c.i += c.freqAddend * c.KI
	freq -= c.freqAddend - (c.freqAddend * c.KI)

	if c.StepThreshold != 0 && offset.Abs() >= c.StepThreshold {
		freq += float64(drift)
		c.freqAddend = 0
	} else {
		c.p = freqOffset(offset) * c.KP
		c.freqAddend = c.p
		c.d = 0.0
		if c.KD != 0.0 {
			c.d = float64(drift) * c.KD
			c.freqAddend += c.d
		}
		freq += c.freqAddend
		offset = 0
	}

	if offset != 0 {
		log.LogAttrs(ctx, slog.LevelDebug, "adjusting clock",
			slog.Duration("offset", offset))
		tx = unix.Timex{
			Modes: unix.ADJ_SETOFFSET | unix.ADJ_NANO,
			Time:  unixutil.NsecToNsecTimeval(offset.Nanoseconds()),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
	}

	log.LogAttrs(ctx, slog.LevelDebug, "adjusting clock frequency",
		slog.Float64("frequency", freq))
	tx = unix.Timex{
		Modes: unix.ADJ_FREQUENCY | unix.ADJ_NANO,
		Freq:  unixutil.FreqToScaledPPM(freq),
	}
	_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
	if err != nil {
		logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
	}
}
