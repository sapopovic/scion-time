package core

import (
	"log"
	"time"
)

type SysClock struct {}

var _ LocalClock = (*SysClock)(nil)

func (c *SysClock) Now() time.Time {
	log.Printf("core.SysClock.Now")
	return time.Time{}
}

func (c *SysClock) Adjust(offset, duration time.Duration, frequency float64) {
	log.Printf("core.SysClock.Adjust")
}

func (c SysClock) Sleep(duration time.Duration) {
	log.Printf("core.SysClock.Sleep")
}
