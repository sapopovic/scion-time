/*
 * Based on flashptpd, https://github.com/meinberg-sync/flashptpd
 *
 * @file adjtimex.h and adjtimex.cpp
 * @note Copyright 2023, Meinberg Funkuhren GmbH & Co. KG, All rights reserved.
 * @author Thomas Behn <thomas.behn@meinberg.de>
 *
 * Adjustment algorithm for the system clock (CLOCK_REALTIME) using the standard
 * Linux API.
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
	"example.com/scion-time/base/timemath"
	"example.com/scion-time/base/unixutil"
)

const (
	//lint:ignore ST1003 maintain consistency with package 'unix'
	unixSTA_RONLY = 65280
)

const (
	adjStepLimit = 500 * time.Millisecond
)

type Adjtimex struct{}

var _ Adjustment = (*Adjtimex)(nil)

func (a *Adjtimex) Do(offset time.Duration, drift float64) {
	ctx := context.Background()
	log := slog.Default()
	tx := unix.Timex{}
	if timemath.Abs(offset) > adjStepLimit {
		log.LogAttrs(ctx, slog.LevelDebug, "stepping clock",
			slog.Duration("offset", offset))
		tx.Modes |= unix.ADJ_SETOFFSET
		tx.Modes |= unix.ADJ_NANO
		tx.Time = unixutil.NsecToNsecTimeval(offset.Nanoseconds())
	} else {
		log.LogAttrs(ctx, slog.LevelDebug, "adjusting clock",
			slog.Duration("offset", offset))
		_, err := unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		tx.Modes |= unix.ADJ_OFFSET
		tx.Modes |= unix.ADJ_STATUS
		tx.Modes |= unix.ADJ_NANO
		tx.Status |= unix.STA_PLL
		tx.Status |= unix.STA_NANO
		tx.Status &= ^unixSTA_RONLY
		tx.Status &= ^unix.STA_FREQHOLD
		tx.Offset = offset.Nanoseconds()
	}
	_, err := unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
	if err != nil {
		logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
	}
}
