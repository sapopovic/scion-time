package core

import (
	"log"
	"time"
)

const sysClockLogPrefix = "[core/clock_sys]"

type SystemClock struct{}

var _ LocalClock = (*SystemClock)(nil)

func (c *SystemClock) Now() time.Time {
	log.Printf("%s core.SystemClock.Now", sysClockLogPrefix)
	return time.Time{}
}

func (c *SystemClock) MaxDrift(duration time.Duration) time.Duration {
	log.Printf("%s core.SystemClock.MaxDrift", sysClockLogPrefix)
	return 0
}

func (c *SystemClock) Step(offset time.Duration) {
	log.Printf("%s core.SystemClock.Step", sysClockLogPrefix)
}

func (c *SystemClock) Adjust(offset, duration time.Duration) {
	log.Printf("%s core.SystemClock.Adjust", sysClockLogPrefix)
}

func (c SystemClock) Sleep(duration time.Duration) {
	log.Printf("%s core.SystemClock.Sleep", sysClockLogPrefix)
}
