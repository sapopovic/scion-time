package core

import (
	"fmt"
	"time"
)

type LocalClock interface {
	Now() time.Time
	MaxDrift(duration time.Duration) time.Duration
	Step(offset time.Duration)
	Adjust(offset, duration time.Duration) // TODO: add argument 'frequency float64'
	Sleep(duration time.Duration)
}

var (
	localClock LocalClock

	errNoClockMeasurements = fmt.Errorf("failed to measure clock values")
)

func LocalClockInstance() LocalClock {
	if localClock == nil {
		panic("no local clock registered")
	}
	return localClock
}

func RegisterLocalClock(c LocalClock) {
	if c == nil {
		panic("local clock must not be nil")
	}
	if localClock != nil {
		panic("local clock already registered")
	}
	localClock = c
}
