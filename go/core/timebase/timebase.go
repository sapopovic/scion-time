package timebase

import (
	"time"
)

type LocalClock interface {
	Epoch() uint64
	Now() time.Time
	MaxDrift(duration time.Duration) time.Duration
	Step(offset time.Duration)
	Adjust(offset, duration time.Duration, frequency float64)
	Sleep(duration time.Duration)
}

var (
	localClock LocalClock
)

func RegisterClock(c LocalClock) {
	if c == nil {
		panic("Local clock must not be nil")
	}
	if localClock != nil {
		panic("Local clock already registered")
	}
	localClock = c
}

func Epoch() uint64 {
	if localClock == nil {
		panic("No local clock registered")
	}
	return localClock.Epoch()
}

func Now() time.Time {
	if localClock == nil {
		panic("No local clock registered")
	}
	return localClock.Now()
}
