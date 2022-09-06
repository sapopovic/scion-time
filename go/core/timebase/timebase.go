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
	lclk LocalClock
)

func RegisterClock(c LocalClock) {
	if c == nil {
		panic("local clock must not be nil")
	}
	if lclk != nil {
		panic("local clock already registered")
	}
	lclk = c
}

func Now() time.Time {
	if lclk == nil {
		panic("no local clock registered")
	}
	return lclk.Now()
}

func Epoch() uint64 {
	if lclk == nil {
		panic("no local clock registered")
	}
	return lclk.Epoch()
}
