package core

import (
	"log"
	"time"
)

type sysClock struct {}

func (c *sysClock) Now() time.Time {
	log.Printf("core.sysClock.Now")
	return time.Time{}
}

func (c *sysClock) Adjust(offset, duration time.Duration, frequency float64) {
	log.Printf("core.sysClock.Adjust")
}

func (c *sysClock) Sleep(duration time.Duration) {
	log.Printf("core.sysClock.Sleep")
}

func NewSysClock() LocalClock {
	return &sysClock{}
}
