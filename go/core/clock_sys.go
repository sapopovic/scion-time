package core

import (
	"log"
	"math"
	"time"

	"example.com/scion-time/go/core/timebase"
)

const sysClockLogPrefix = "[core/clock_sys]"

type SystemClock struct{}

var _ timebase.LocalClock = (*SystemClock)(nil)

func (c *SystemClock) Now() time.Time {
	log.Printf("%s core.SystemClock.Now()", sysClockLogPrefix)
	return time.Now().UTC()
}

func (c *SystemClock) MaxDrift(duration time.Duration) time.Duration {
	log.Printf("%s core.SystemClock.MaxDrift(%v)", sysClockLogPrefix, duration)
	return math.MaxInt64
}

func (c *SystemClock) Step(offset time.Duration) {
	log.Printf("%s core.SystemClock.Step(%v)", sysClockLogPrefix, offset)
}

func (c *SystemClock) Adjust(offset, duration time.Duration) {
	log.Printf("%s core.SystemClock.Adjust(%v, %v)", sysClockLogPrefix, offset, duration)
}

func (c SystemClock) Sleep(duration time.Duration) {
	log.Printf("%s core.SystemClock.Sleep(%v)", sysClockLogPrefix, duration)
	time.Sleep(duration)
}
