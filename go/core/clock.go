package core

import (
	"time"
)

type LocalClock interface {
	Now() time.Time
	Adjust(offset, duration time.Duration, frequency float64)
	Sleep(duration time.Duration)
}

var localClock LocalClock

func LocalClockInstance() LocalClock {
	if localClock == nil {
		panic("No local clock registered")
	}
	return localClock
}

func RegisterLocalClock(c LocalClock) {
	if c == nil {
		panic("Local clock must not be nil")
	}
	if localClock != nil {
		panic("Local clock already registered")
	}
	localClock = c
}
