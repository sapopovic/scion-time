//go:build !linux

package core

import (
	"log"
	"math"
	"time"

	"example.com/scion-time/go/core/timebase"
)

const sysClockLogPrefix = "[core/clock_sys_std]"

type SystemClock struct{}

var _ timebase.LocalClock = (*SystemClock)(nil)

func (c *SystemClock) Epoch() uint64 {
	return 0
}

func (c *SystemClock) Now() time.Time {
	return time.Now().UTC()
}

func (c *SystemClock) MaxDrift(duration time.Duration) time.Duration {
	return math.MaxInt64
}

func (c *SystemClock) Step(offset time.Duration) {
	log.Printf("%s core.SystemClock.Step(%v)", sysClockLogPrefix, offset)
}

func (c *SystemClock) Adjust(offset, duration time.Duration, frequency float64) {
	log.Printf("%s core.SystemClock.Adjust(%v, %v, %v)", sysClockLogPrefix, offset, duration, frequency)
}

func (c SystemClock) Sleep(duration time.Duration) {
	log.Printf("%s core.SystemClock.Sleep(%v)", sysClockLogPrefix, duration)
	time.Sleep(duration)
}
