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

func (c *SysClock) Adjust(offset, duration time.Duration, frequency float64) {
	log.Printf("%s core.SysClock.Adjust", sysClockLogPrefix)
}

func (c SysClock) Sleep(duration time.Duration) {
	log.Printf("%s core.SysClock.Sleep", sysClockLogPrefix)
}
