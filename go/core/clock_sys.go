package core

import (
	"log"
	"time"
)

const sysClockLogPrefix = "[core/clock_sys]"

type SysClock struct{}

var _ LocalClock = (*SysClock)(nil)

func (c *SysClock) Now() time.Time {
	log.Printf("%s core.SysClock.Now", sysClockLogPrefix)
	return time.Time{}
}

func (c *SysClock) MaxDrift(duration time.Duration) time.Duration {
	log.Printf("%s core.SysClock.MaxDrift", sysClockLogPrefix)
	return 0
}

func (c *SysClock) Step(offset time.Duration) {
	log.Printf("%s core.SysClock.Step", sysClockLogPrefix)
}

func (c *SysClock) Adjust(offset, duration time.Duration) {
	log.Printf("%s core.SysClock.Adjust", sysClockLogPrefix)
}

func (c SysClock) Sleep(duration time.Duration) {
	log.Printf("%s core.SysClock.Sleep", sysClockLogPrefix)
}
